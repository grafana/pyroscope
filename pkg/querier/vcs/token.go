package vcs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

const (
	gitHubCookieName = "GitSession"
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

func tokenFromRequest(req connect.AnyRequest) (*oauth2.Token, error) {
	cookie, err := (&http.Request{Header: req.Header()}).Cookie(gitHubCookieName)
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
		Name:     gitHubCookieName,
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
