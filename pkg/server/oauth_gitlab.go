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

type oauthHanlderGitlab struct {
	oauthBase
	allowedGroups []string
}

func newGitlabHandler(cfg config.GitlabOauth, baseURL string, log *logrus.Logger) (*oauthHanlderGitlab, error) {
	authURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return nil, err
	}

	h := &oauthHanlderGitlab{
		oauthBase: oauthBase{
			config: &oauth2.Config{
				ClientID:     cfg.ClientID,
				ClientSecret: cfg.ClientSecret,
				Scopes:       []string{"read_api"},
				Endpoint:     oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL},
			},
			authURL:       authURL,
			log:           log,
			callbackRoute: "/auth/gitlab/callback",
			redirectRoute: "/auth/gitlab/redirect",
			apiURL:        cfg.APIURL,
			baseURL:       baseURL,
		},
		allowedGroups: cfg.AllowedGroups,
	}

	if cfg.RedirectURL != "" {
		h.config.RedirectURL = cfg.RedirectURL
	}

	return h, nil
}

type gitlabGroups struct {
	Path string
}

func (o oauthHanlderGitlab) userAuth(client *http.Client) (*externalUser, error) {
	type userProfileResponse struct {
		ID        int64
		Email     string
		Username  string
		AvatarURL string
	}

	resp, err := client.Get(o.oauthBase.apiURL + "/user")
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
		Name:  userProfile.Username,
		Email: userProfile.Email,
	}
	if len(o.allowedGroups) == 0 {
		return &u, nil
	}

	groups, err := o.fetchGroups(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	for _, allowed := range o.allowedGroups {
		for _, member := range groups {
			if member.Path == allowed {
				return &u, nil
			}
		}
	}

	return nil, errForbidden
}

func (o oauthHanlderGitlab) fetchGroups(client *http.Client) ([]gitlabGroups, error) {
	groupsURL := o.apiURL + "/groups"
	more := true
	groups := make([]gitlabGroups, 0)

	for more {
		resp, err := client.Get(groupsURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var grp []gitlabGroups
		err = json.NewDecoder(resp.Body).Decode(&grp)
		if err != nil {
			return nil, err
		}

		groups = append(groups, grp...)

		groupsURL, more = hasMoreLinkResults(resp.Header)
		if err != nil {
			return nil, err
		}
	}

	return groups, nil
}

func (o oauthHanlderGitlab) getOauthBase() oauthBase {
	return o.oauthBase
}
