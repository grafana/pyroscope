package vcs

import (
	"net/http"
	"os"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
)

const (
	gitHubCookieName = "GitSession"
)

var (
	githubAppClientID     = os.Getenv("GITHUB_CLIENT_ID")
	githubAppClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
	githubSessionSecret   = []byte(os.Getenv("GITHUB_SESSION_SECRET"))
)

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

	cookie := http.Cookie{
		Name:     gitHubCookieName,
		Value:    encrypted,
		Expires:  token.Expiry,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	return cookie.String(), nil
}

func decodeToken(value string) (*oauth2.Token, error) {
	token, err := decryptToken(value, githubSessionSecret)
	if err != nil {
		return nil, err
	}
	return token, nil
}
