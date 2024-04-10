package vcs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

const (
	githubCookieName = "GitSession"
	githubRefreshURL = "https://github.com/login/oauth/access_token"
)

var (
	githubAppClientID     = os.Getenv("GITHUB_CLIENT_ID")
	githubAppClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	githubSessionSecret   = []byte(os.Getenv("GITHUB_SESSION_SECRET"))
)

type gitSessionTokenCookie struct {
	Metadata        string `json:"metadata"`
	ExpiryTimestamp int64  `json:"expiry"`
}

type githubAuthToken struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
	TokenType             string `json:"token_type"`
}

func (t githubAuthToken) toOAuthToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		TokenType:    t.TokenType,
		RefreshToken: t.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(t.ExpiresIn) * time.Second),
	}
}

func githubOAuthConfig() (*oauth2.Config, error) {
	if githubAppClientID == "" {
		return nil, fmt.Errorf("missing GITHUB_CLIENT_ID environment variable")
	}
	if githubAppClientSecret == "" {
		return nil, fmt.Errorf("missing GITHUB_CLIENT_SECRET environment variable")
	}
	return &oauth2.Config{
		ClientID:     githubAppClientID,
		ClientSecret: githubAppClientSecret,
		Endpoint:     endpoints.GitHub,
	}, nil
}

func refreshToken(ctx context.Context, oldToken *oauth2.Token) (*oauth2.Token, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", githubRefreshURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.URL.RawQuery = buildRefreshQuery(req.URL.Query(), oldToken).Encode()

	client := http.Client{
		Timeout: 10 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer req.Body.Close()

	githubToken := githubAuthToken{}
	err = json.NewDecoder(res.Body).Decode(&githubToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	newToken := githubToken.toOAuthToken()
	return newToken, nil
}

func buildRefreshQuery(query url.Values, token *oauth2.Token) url.Values {
	query.Add("client_id", githubAppClientID)
	query.Add("client_secret", githubAppClientSecret)
	query.Add("grant_type", "refresh_token")
	query.Add("refresh_token", token.RefreshToken)
	return query
}

func tokenFromRequest(req connect.AnyRequest) (*oauth2.Token, error) {
	cookie, err := (&http.Request{Header: req.Header()}).Cookie(githubCookieName)
	if err != nil {
		return nil, err
	}

	token, err := decodeToken(cookie.Value)
	if err != nil {
		return nil, err
	}

	return token, nil
}

func encodeToken(token *oauth2.Token) (string, error) {
	encrypted, err := encryptToken(token, githubSessionSecret)
	if err != nil {
		return "", err
	}

	bytes, err := json.Marshal(gitSessionTokenCookie{
		Metadata:        encrypted,
		ExpiryTimestamp: token.Expiry.UnixMilli(),
	})
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(bytes)
	cookie := http.Cookie{
		Name:     githubCookieName,
		Value:    encoded,
		Expires:  token.Expiry,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	return cookie.String(), nil
}

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
		token, innerErr = decryptToken(value, githubSessionSecret)
		if innerErr != nil {
			// Legacy fallback failed, return the original error.
			return nil, err
		}
		return token, nil
	}

	token, err = decryptToken(sessionToken.Metadata, githubSessionSecret)
	if err != nil {
		return nil, err
	}
	return token, nil
}
