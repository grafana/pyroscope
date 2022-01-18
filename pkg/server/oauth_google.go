package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

type oauthHandlerGoogle struct {
	oauthBase
	allowedDomains []string
}

func newOauthGoogleHandler(cfg config.GoogleOauth, baseURL string, log *logrus.Logger) (*oauthHandlerGoogle, error) {
	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return nil, err
	}

	h := &oauthHandlerGoogle{
		oauthBase: oauthBase{
			config: &oauth2.Config{
				ClientID:     cfg.ClientID,
				ClientSecret: cfg.ClientSecret,
				Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
				Endpoint:     oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL},
			},
			authURL:       authURL,
			log:           log,
			callbackRoute: "/auth/google/callback",
			redirectRoute: "/auth/google/redirect",
			apiURL:        "https://www.googleapis.com/oauth2/v2",
			baseURL:       baseURL,
		},
		allowedDomains: cfg.AllowedDomains,
	}

	if cfg.RedirectURL != "" {
		h.config.RedirectURL = cfg.RedirectURL
	}

	return h, nil
}

func (o oauthHandlerGoogle) userAuth(client *http.Client) (string, error) {
	type userProfileResponse struct {
		ID            string
		Email         string
		VerifiedEmail bool
		Picture       string
	}

	resp, err := client.Get(o.apiURL + "/userinfo")
	if err != nil {
		return "", fmt.Errorf("failed to get oauth user info: %w", err)
	}
	defer resp.Body.Close()

	var userProfile userProfileResponse
	err = json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", fmt.Errorf("failed to decode user profile response: %w", err)
	}

	if userProfile.Email == "" {
		return "", errors.New("user email is empty")
	}

	if len(o.allowedDomains) == 0 || (len(o.allowedDomains) > 0 && isAllowedDomain(o.allowedDomains, userProfile.Email)) {
		return userProfile.Email, nil
	}

	return "", errForbidden
}

func isAllowedDomain(allowedDomains []string, email string) bool {
	for _, domain := range allowedDomains {
		if strings.HasSuffix(email, fmt.Sprintf("@%s", domain)) {
			return true
		}
	}

	return false
}

func (o oauthHandlerGoogle) getOauthBase() oauthBase {
	return o.oauthBase
}
