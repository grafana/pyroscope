package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

var errForbidden = errors.New("access forbidden")

type extUserInfo struct {
	Name  string
	Email string
}

type oauthHandler interface {
	userAuth(client *http.Client) (extUserInfo, error)
	getOauthBase() oauthBase
}

type oauthBase struct {
	config        *oauth2.Config
	authURL       *url.URL
	apiURL        string
	log           *logrus.Logger
	callbackRoute string
	redirectRoute string
	baseURL       string
}

func (o oauthBase) getCallbackURL(host, configCallbackURL string, hasTLS bool) (string, error) {
	// I don't think this is ever true... but not super sure
	if configCallbackURL != "" {
		return configCallbackURL, nil
	}

	schema := "http"
	if hasTLS {
		schema = "https"
	}

	if o.baseURL != "" {
		u, err := url.Parse(o.baseURL)
		if err != nil {
			return "", err
		}
		if u.Scheme == "" {
			u.Scheme = schema
		}
		if u.Host == "" {
			u.Host = host
		}
		u.Path = filepath.Join(u.Path, o.callbackRoute)
		return u.String(), nil
	}

	if host == "" {
		return "", errors.New("host is empty")
	}

	return fmt.Sprintf("%v://%v%v", schema, host, o.callbackRoute), nil
}

func (o oauthBase) buildAuthQuery(r *http.Request, w http.ResponseWriter) (redirectURL string, state string, err error) {
	callbackURL, err := o.getCallbackURL(r.Host, o.config.RedirectURL, r.URL.Query().Get("tls") == "true")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return "", "", fmt.Errorf("callbackURL parsing failed: %w", err)
	}

	authURL := *o.authURL
	parameters := url.Values{}
	parameters.Add("client_id", o.config.ClientID)
	parameters.Add("scope", strings.Join(o.config.Scopes, " "))
	parameters.Add("redirect_uri", callbackURL)
	parameters.Add("response_type", "code")

	// generate state token for CSRF protection
	if state, err = generateStateToken(16); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return "", "", fmt.Errorf("problem generating state token: %w", err)
	}

	parameters.Add("state", state)
	authURL.RawQuery = parameters.Encode()
	return authURL.String(), state, nil
}

func (o oauthBase) generateOauthClient(r *http.Request) (*http.Client, error) {
	code := r.FormValue("code")
	if code == "" {
		return nil, errors.New("code not found")
	}

	callbackURL, err := o.getCallbackURL(r.Host, o.config.RedirectURL, r.URL.Query().Get("tls") == "true")
	if err != nil {
		return nil, fmt.Errorf("callbackURL parsing failed: %w", err)
	}
	oauthConf := *o.config
	oauthConf.RedirectURL = callbackURL
	token, err := oauthConf.Exchange(r.Context(), code)
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code for token failed: %w", err)
	}

	return oauthConf.Client(r.Context(), token), err
}

func hasMoreLinkResults(headers http.Header) (string, bool) {
	value, exists := headers["Link"]
	if !exists {
		return "", false
	}

	pattern := regexp.MustCompile(`<([^>]+)>; rel="next"`)
	matches := pattern.FindStringSubmatch(value[0])
	if matches == nil {
		return "", false
	}

	next := matches[1]

	return next, true
}
