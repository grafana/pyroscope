package vcs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"

	"github.com/grafana/pyroscope/pkg/tenant"
)

const (
	sessionCookieName = "pyroscope_git_session"
)

// Deprecated: this is the old format for encoded token inside a cookie
// Remove after completing https://github.com/grafana/explore-profiles/issues/187
type deprecatedGitSessionTokenCookie struct {
	Metadata        string `json:"metadata"`
	ExpiryTimestamp int64  `json:"expiry"`
}

type gitSessionTokenCookie struct {
	Token *string `json:"token"`
}

const envVarGithubSessionSecret = "GITHUB_SESSION_SECRET"

var githubSessionSecret = []byte(os.Getenv(envVarGithubSessionSecret))

// derives a per tenant key from the global session secret using sha256
func deriveEncryptionKeyForContext(ctx context.Context) ([]byte, error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(tenantID) == 0 {
		return nil, errors.New("tenantID is empty")
	}

	if len(githubSessionSecret) == 0 {
		return nil, errors.New(envVarGithubSessionSecret + " is empty")
	}
	h := sha256.New()
	h.Write(githubSessionSecret)
	h.Write([]byte{':'})
	h.Write([]byte(tenantID))
	return h.Sum(nil), nil
}

// getStringValueFrom gets a string value from url.Values. It will fail if the
// key is missing or the key's value is an empty string.
func getStringValueFrom(values url.Values, key string) (string, error) {
	value := values.Get(key)
	if value == "" {
		return "", fmt.Errorf("missing key: %s", key)
	}
	return value, nil
}

// getDurationValueFrom gets a duration value from url.Values. It will fail if
// the key is missing, the key's value is an empty string, or the key's value
// cannot be parsed into a duration.
func getDurationValueFrom(values url.Values, key string, scalar time.Duration) (time.Duration, error) {
	if scalar < 1 {
		return 0, fmt.Errorf("cannot use scalar less than 1")
	}

	value, err := getStringValueFrom(values, key)
	if err != nil {
		return 0, err
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", key, err)
	}

	return time.Duration(n) * scalar, nil
}

// tokenFromRequest decodes an OAuth token from a request.
func tokenFromRequest(ctx context.Context, req connect.AnyRequest) (*oauth2.Token, error) {
	cookie, err := (&http.Request{Header: req.Header()}).Cookie(sessionCookieName)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookie %s: %w", sessionCookieName, err)
	}

	derivedKey, err := deriveEncryptionKeyForContext(ctx)
	if err != nil {
		return nil, err
	}

	token, err := decodeToken(cookie.Value, derivedKey)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// Deprecated: encodeTokenInCookie creates a cookie by encrypting then base64 encoding an OAuth token.
// In future version, the cookie that this function creates will be no longer sent by the backend.
// Instead, backend provides the necessary data so frontend can create its own GitHub session cookie.
// Remove after completing https://github.com/grafana/explore-profiles/issues/187
func encodeTokenInCookie(token *oauth2.Token, key []byte) (*http.Cookie, error) {
	encrypted, err := encryptToken(token, key)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(deprecatedGitSessionTokenCookie{
		Metadata:        encrypted,
		ExpiryTimestamp: token.Expiry.UnixMilli(),
	})
	if err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(bytes)
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    encoded,
		Expires:  time.Now().Add(githubRefreshExpiryDuration),
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	return cookie, nil
}

// decodeToken base64 decodes and decrypts a OAuth token.
func decodeToken(value string, key []byte) (*oauth2.Token, error) {
	var token *oauth2.Token

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}

	sessionToken := gitSessionTokenCookie{}
	err = json.Unmarshal(decoded, &sessionToken)
	if err != nil || sessionToken.Token == nil {
		// This may be a deprecated cookie. Deprecated cookies are base64 encoded deprecatedGitSessionTokenCookie objects.
		token, innerErr := decodeDeprecatedToken(decoded, key)
		if innerErr != nil {
			// Deprecated fallback failed, return the original error.
			return nil, err
		}
		return token, nil
	}

	token, err = decryptToken(*sessionToken.Token, key)
	if err != nil {
		return nil, err
	}

	return token, nil
}

// Deprecated: decodeDeprecatedToken decrypts a deprecatedGitSessionTokenCookie
// In future version, frontend won't send any deprecated cookies.
// Remove alongside encodeTokenInCookie, after completing https://github.com/grafana/explore-profiles/issues/187
func decodeDeprecatedToken(value []byte, key []byte) (*oauth2.Token, error) {
	var token *oauth2.Token

	sessionToken := &deprecatedGitSessionTokenCookie{}
	err := json.Unmarshal(value, sessionToken)
	if err != nil || sessionToken == nil {
		return nil, err
	}

	token, err = decryptToken(sessionToken.Metadata, key)
	if err != nil {
		return nil, err
	}
	return token, nil
}
