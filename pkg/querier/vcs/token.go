package vcs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "GitSession"
)

var (
	gitSessionSecret = []byte(os.Getenv("GITHUB_SESSION_SECRET"))
)

type gitSessionTokenCookie struct {
	Metadata        string `json:"metadata"`
	ExpiryTimestamp int64  `json:"expiry"`
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
func tokenFromRequest(req connect.AnyRequest) (*oauth2.Token, error) {
	cookie, err := (&http.Request{Header: req.Header()}).Cookie(sessionCookieName)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookie %s: %w", sessionCookieName, err)
	}

	token, err := decodeToken(cookie.Value)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// encodeToken encrypts then base64 encodes an OAuth token.
func encodeToken(token *oauth2.Token) (*http.Cookie, error) {
	encrypted, err := encryptToken(token, gitSessionSecret)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(gitSessionTokenCookie{
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
func decodeToken(value string) (*oauth2.Token, error) {
	var token *oauth2.Token

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}

	sessionToken := gitSessionTokenCookie{}
	err = json.Unmarshal(decoded, &sessionToken)
	if err != nil {
		// This may be a legacy cookie. Legacy cookies aren't base64 encoded
		// JSON objects, but rather a base64 encoded crypto hash.
		var innerErr error
		token, innerErr = decryptToken(value, gitSessionSecret)
		if innerErr != nil {
			// Legacy fallback failed, return the original error.
			return nil, err
		}
		return token, nil
	}

	token, err = decryptToken(sessionToken.Metadata, gitSessionSecret)
	if err != nil {
		return nil, err
	}
	return token, nil
}
