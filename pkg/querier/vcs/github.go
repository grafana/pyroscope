package vcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

const (
	githubRefreshURL = "https://github.com/login/oauth/access_token"

	// Duration of a GitHub refresh token. The original OAuth flow doesn't
	// return the refresh token expiry, so we need to store it separately.
	// GitHub docs state this value will never change:
	//
	// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens
	githubRefreshExpiryDuration = 15897600 * time.Second
)

var (
	githubAppClientID     = os.Getenv("GITHUB_CLIENT_ID")
	githubAppClientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
)

type githubAuthToken struct {
	AccessToken           string        `json:"access_token"`
	ExpiresIn             time.Duration `json:"expires_in"`
	RefreshToken          string        `json:"refresh_token"`
	RefreshTokenExpiresIn time.Duration `json:"refresh_token_expires_in"`
	Scope                 string        `json:"scope"`
	TokenType             string        `json:"token_type"`
}

// toOAuthToken converts a githubAuthToken to an OAuth token.
func (t githubAuthToken) toOAuthToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		TokenType:    t.TokenType,
		RefreshToken: t.RefreshToken,
		Expiry:       time.Now().Add(t.ExpiresIn),
	}
}

// githubOAuthConfig creates a GitHub OAuth config.
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

// refreshGithubToken sends a request configured for the GitHub API and marshals
// the response into a githubAuthToken.
func refreshGithubToken(req *http.Request, client *http.Client) (*githubAuthToken, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer res.Body.Close()

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// The response body is application/x-www-form-urlencoded, so we parse it
	// via url.ParseQuery.
	payload, err := url.ParseQuery(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse response body: %w", err)
	}

	githubToken, err := githubAuthTokenFromFormURLEncoded(payload)
	if err != nil {
		return nil, err
	}

	return githubToken, nil
}

// buildGithubRefreshRequest builds a cancelable http.Request which is
// configured to hit the GitHub API's token refresh endpoint.
func buildGithubRefreshRequest(ctx context.Context, oldToken *oauth2.Token) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", githubRefreshURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("client_id", githubAppClientID)
	query.Add("client_secret", githubAppClientSecret)
	query.Add("grant_type", "refresh_token")
	query.Add("refresh_token", oldToken.RefreshToken)

	req.URL.RawQuery = query.Encode()
	return req, nil
}

// githubAuthTokenFromFormURLEncoded converts a url-encoded form to a
// githubAuthToken.
func githubAuthTokenFromFormURLEncoded(values url.Values) (*githubAuthToken, error) {
	token := &githubAuthToken{}
	var err error

	token.AccessToken, err = getStringValueFrom(values, "access_token")
	if err != nil {
		return nil, err
	}

	token.ExpiresIn, err = getDurationValueFrom(values, "expires_in", time.Second)
	if err != nil {
		return nil, err
	}

	token.RefreshToken, err = getStringValueFrom(values, "refresh_token")
	if err != nil {
		return nil, err
	}

	token.RefreshTokenExpiresIn, err = getDurationValueFrom(values, "refresh_token_expires_in", time.Second)
	if err != nil {
		return nil, err
	}

	token.Scope, err = getStringValueFrom(values, "scope")
	if err != nil {
		return nil, err
	}

	token.TokenType, err = getStringValueFrom(values, "token_type")
	if err != nil {
		return nil, err
	}

	return token, nil
}

func isGitHubIntegrationConfigured() error {
	var errs []error

	if githubAppClientID == "" {
		errs = append(errs, fmt.Errorf("missing GITHUB_CLIENT_ID environment variable"))
	}

	if githubAppClientSecret == "" {
		errs = append(errs, fmt.Errorf("missing GITHUB_CLIENT_SECRET environment variable"))
	}

	if len(githubSessionSecret) == 0 {
		errs = append(errs, fmt.Errorf("missing GITHUB_SESSION_SECRET environment variable"))
	}

	return errors.Join(errs...)
}
