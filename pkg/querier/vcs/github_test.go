package vcs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func Test_githubAuthToken_toOAuthToken(t *testing.T) {
	gat := githubAuthToken{
		AccessToken:           "my_access_token",
		ExpiresIn:             60 * time.Second, // 1 minute
		RefreshToken:          "my_refresh_token",
		RefreshTokenExpiresIn: 120 * time.Second, // 2 minutes
		Scope:                 "refresh_token",
		TokenType:             "bearer",
	}

	want := &oauth2.Token{
		AccessToken:  "my_access_token",
		TokenType:    "bearer",
		RefreshToken: "my_refresh_token",
		Expiry:       time.Now().Add(gat.ExpiresIn),
	}

	got := gat.toOAuthToken()

	require.GreaterOrEqual(t, got.Expiry.UnixMilli(), want.Expiry.UnixMilli())
	got.Expiry = want.Expiry

	require.Equal(t, want, got)
}

func Test_refreshGithubToken(t *testing.T) {
	want := githubAuthToken{
		AccessToken:           "my_access_token",
		ExpiresIn:             60 * time.Second, // 1 minute
		RefreshToken:          "my_refresh_token",
		RefreshTokenExpiresIn: 120 * time.Second, // 2 minutes
		Scope:                 "refresh_token",
		TokenType:             "bearer",
	}
	fakeGithubAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		query := (&url.URL{}).Query()
		query.Add("access_token", want.AccessToken)
		query.Add("expires_in", fmt.Sprintf("%d", int(want.ExpiresIn.Seconds())))
		query.Add("refresh_token", want.RefreshToken)
		query.Add("refresh_token_expires_in", fmt.Sprintf("%d", int(want.RefreshTokenExpiresIn.Seconds())))
		query.Add("scope", want.Scope)
		query.Add("token_type", want.TokenType)

		_, err := w.Write([]byte(query.Encode()))
		require.NoError(t, err)
	}))
	defer fakeGithubAPI.Close()

	req, err := http.NewRequest("POST", fakeGithubAPI.URL, nil)
	require.NoError(t, err)

	got, err := refreshGithubToken(req, http.DefaultClient)
	require.NoError(t, err)
	require.Equal(t, want, *got)
}

func Test_buildGithubRefreshRequest(t *testing.T) {
	oldToken := &oauth2.Token{
		AccessToken:  "my_access_token",
		TokenType:    "my_token_type",
		RefreshToken: "my_refresh_token",
		Expiry:       time.Unix(1713298947, 0), // 2024-04-16T20:22:27.346Z
	}

	// Override env vars.
	githubAppClientID = "my_github_client_id"
	githubAppClientSecret = "my_github_client_secret"

	got, err := buildGithubRefreshRequest(context.Background(), oldToken)
	require.NoError(t, err)

	wantQuery := url.Values{
		"client_id":     {githubAppClientID},
		"client_secret": {githubAppClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {oldToken.RefreshToken},
	}
	require.Equal(t, nil, got.Body)
	require.Equal(t, wantQuery, got.URL.Query())

	wantURL, err := url.Parse(githubRefreshURL)
	require.NoError(t, err)
	require.Equal(t, wantURL.Host, got.URL.Host)
	require.Equal(t, wantURL.Path, got.URL.Path)
}

func Test_githubAuthTokenFromFormURLEncoded(t *testing.T) {
	tests := []struct {
		Name       string
		Query      url.Values
		Want       *githubAuthToken
		WantErrMsg string
	}{
		{
			Name: "valid query",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"60"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			Want: &githubAuthToken{
				AccessToken:           "my_access_token",
				ExpiresIn:             60 * time.Second,
				RefreshToken:          "my_refresh_token",
				RefreshTokenExpiresIn: 120 * time.Second,
				Scope:                 "refresh_token",
				TokenType:             "bearer",
			},
		},
		{
			Name: "missing access_token",
			Query: url.Values{
				"expires_in":               {"60"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "missing key: access_token",
		},
		{
			Name: "missing expires_in",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "missing key: expires_in",
		},
		{
			Name: "missing refresh_token",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"60"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "missing key: refresh_token",
		},
		{
			Name: "missing refresh_token_expires_in",
			Query: url.Values{
				"access_token":  {"my_access_token"},
				"expires_in":    {"60"},
				"refresh_token": {"my_refresh_token"},
				"scope":         {"refresh_token"},
				"token_type":    {"bearer"},
			},
			WantErrMsg: "missing key: refresh_token_expires_in",
		},
		{
			Name: "missing scope",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"60"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "missing key: scope",
		},
		{
			Name: "missing token_type",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"60"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
			},
			WantErrMsg: "missing key: token_type",
		},
		{
			Name: "invalid expires_in",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"not_a_number"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"120"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "failed to parse expires_in: strconv.Atoi: parsing \"not_a_number\": invalid syntax",
		},
		{
			Name: "invalid refresh_token_expires_in",
			Query: url.Values{
				"access_token":             {"my_access_token"},
				"expires_in":               {"60"},
				"refresh_token":            {"my_refresh_token"},
				"refresh_token_expires_in": {"not_a_number"},
				"scope":                    {"refresh_token"},
				"token_type":               {"bearer"},
			},
			WantErrMsg: "failed to parse refresh_token_expires_in: strconv.Atoi: parsing \"not_a_number\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gat, err := githubAuthTokenFromFormURLEncoded(tt.Query)
			if tt.WantErrMsg != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want, gat)
			}
		})
	}
}
