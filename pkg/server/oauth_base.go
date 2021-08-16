package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

var errForbidden = errors.New("access forbidden")

type oauthHandler interface {
	userAuth(client *http.Client) (string, error)
	getOauthBase() oauthBase
}

type oauthBase struct {
	config        *oauth2.Config
	authURL       *url.URL
	apiURL        string
	log           *logrus.Logger
	callbackRoute string
	redirectRoute string
}

func (o oauthBase) getCallbackURL(host, configCallbackURL string, hasTLS bool) (string, error) {
	if configCallbackURL != "" {
		return configCallbackURL, nil
	}

	if host == "" {
		return "", errors.New("host is empty")
	}

	schema := "http"
	if hasTLS {
		schema = "https"
	}

	return fmt.Sprintf("%v://%v/%v", schema, host, o.callbackRoute), nil
}

func (o oauthBase) buildAuthQuery(r *http.Request, w http.ResponseWriter) (string, error) {
	callbackURL, err := o.getCallbackURL(r.Host, o.config.RedirectURL, r.URL.Query().Get("tls") == "true")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return "", fmt.Errorf("callbackURL parsing failed: %w", err)
	}

	authURL := *o.authURL
	parameters := url.Values{}
	parameters.Add("client_id", o.config.ClientID)
	parameters.Add("scope", strings.Join(o.config.Scopes, " "))
	parameters.Add("redirect_uri", callbackURL)
	parameters.Add("response_type", "code")

	// generate state token for CSRF protection
	state, err := generateStateToken(16)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return "", fmt.Errorf("problem generating state token: %w", err)
	}

	createCookie(w, stateCookieName, state)
	parameters.Add("state", state)
	authURL.RawQuery = parameters.Encode()

	return authURL.String(), nil
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
