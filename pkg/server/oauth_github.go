package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

type oauthHanlderGithub struct {
	oauthBase
	allowedOrganizations []string
}

func newGithubHandler(cfg config.GithubOauth, baseURL string, log *logrus.Logger) (*oauthHanlderGithub, error) {
	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return nil, err
	}

	h := &oauthHanlderGithub{
		oauthBase: oauthBase{
			config: &oauth2.Config{
				ClientID:     cfg.ClientID,
				ClientSecret: cfg.ClientSecret,
				Scopes:       []string{"read:user", "user:email", "read:org"},
				Endpoint:     oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL},
			},
			authURL:       authURL,
			log:           log,
			callbackRoute: "/auth/github/callback",
			redirectRoute: "/auth/github/redirect",
			apiURL:        "https://api.github.com",
			baseURL:       baseURL,
		},
		allowedOrganizations: cfg.AllowedOrganizations,
	}

	if cfg.RedirectURL != "" {
		h.config.RedirectURL = cfg.RedirectURL
	}

	return h, nil
}

type githubOrganizations struct {
	Login string
}

func (o oauthHanlderGithub) userAuth(client *http.Client) (*externalUser, error) {
	type userProfileResponse struct {
		ID        int64
		Email     string
		Login     string
		AvatarURL string
	}

	resp, err := client.Get(o.apiURL + "/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth user info: %w", err)
	}
	defer resp.Body.Close()

	var userProfile userProfileResponse
	err = json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode user profile response: %w", err)
	}

	u := externalUser{
		Name:  userProfile.Login,
		Email: userProfile.Email,
	}
	if len(o.allowedOrganizations) == 0 {
		return &u, nil
	}

	organizations, err := o.fetchOrganizations(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %w", err)
	}

	for _, allowed := range o.allowedOrganizations {
		for _, member := range organizations {
			if member.Login == allowed {
				return &u, nil
			}
		}
	}

	return nil, errForbidden
}

func (o oauthHanlderGithub) fetchOrganizations(client *http.Client) ([]githubOrganizations, error) {
	orgsURL := o.apiURL + "/user/orgs"
	more := true
	organizations := make([]githubOrganizations, 0)

	for more {
		resp, err := client.Get(orgsURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var orgs []githubOrganizations
		err = json.NewDecoder(resp.Body).Decode(&orgs)
		if err != nil {
			return nil, err
		}

		organizations = append(organizations, orgs...)

		orgsURL, more = hasMoreLinkResults(resp.Header)
		if err != nil {
			return nil, err
		}
	}

	return organizations, nil
}

func (o oauthHanlderGithub) getOauthBase() oauthBase {
	return o.oauthBase
}
