package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
)

func (ctrl *Controller) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := ctrl.getTemplate("/login.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render login page")
			return
		}
		mustExecute(tmpl, w, map[string]interface{}{
			"GoogleEnabled": ctrl.config.Google.Enabled,
			"GithubEnabled": ctrl.config.Github.Enabled,
			"GitlabEnabled": ctrl.config.Gitlab.Enabled,
			"BaseURL":       ctrl.config.BaseURL,
		})
	}
}

func createCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: http.SameSiteStrictMode,
	})
}

func invalidateCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
	})
}

func (ctrl *Controller) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodGet:
			invalidateCookie(w, jwtCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		default:
			ctrl.writeErrorMessage(w, http.StatusMethodNotAllowed, "only POST and DELETE methods are allowed")
		}
	}
}

// can be replaced with a faster solution if cryptographic randomness isn't a priority
func generateStateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func getCallbackURL(host, configCallbackURL string, oauthType int, hasTLS bool) (string, error) {
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

	switch oauthType {
	case oauthGoogle:
		return fmt.Sprintf("%v://%v/auth/google/callback", schema, host), nil
	case oauthGithub:
		return fmt.Sprintf("%v://%v/auth/github/callback", schema, host), nil
	case oauthGitlab:
		return fmt.Sprintf("%v://%v/auth/gitlab/callback", schema, host), nil
	}

	return "", errors.New("invalid oauth type provided")
}

func (ctrl *Controller) oauthLoginHandler(info *oauthInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callbackURL, err := getCallbackURL(r.Host, info.Config.RedirectURL, info.Type, r.URL.Query().Get("tls") == "true")
		if err != nil {
			ctrl.log.WithError(err).Error("callbackURL parsing failed")
			return
		}

		authURL := *info.AuthURL
		parameters := url.Values{}
		parameters.Add("client_id", info.Config.ClientID)
		parameters.Add("scope", strings.Join(info.Config.Scopes, " "))
		parameters.Add("redirect_uri", callbackURL)
		parameters.Add("response_type", "code")

		// generate state token for CSRF protection
		state, err := generateStateToken(16)
		if err != nil {
			ctrl.log.WithError(err).Error("problem generating state token")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		createCookie(w, stateCookieName, state)
		parameters.Add("state", state)
		authURL.RawQuery = parameters.Encode()

		http.Redirect(w, r, authURL.String(), http.StatusTemporaryRedirect)
	}
}

// Instead of this handler that just redirects, Javascript code can be added to load the state and send it to backend
// this is done so that the state cookie would be send back from browser
func (ctrl *Controller) callbackHandler(redirectURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := ctrl.getTemplate("/redirect.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render redirect page")
			return
		}
		mustExecute(tmpl, w, map[string]interface{}{
			"RedirectURL": redirectURL + "?" + r.URL.RawQuery,
			"BaseURL":     ctrl.config.BaseURL,
		})
	}
}

func (ctrl *Controller) forbiddenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := ctrl.getTemplate("/forbidden.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render forbidden page")
			return
		}
		mustExecute(tmpl, w, map[string]interface{}{
			"BaseURL": ctrl.config.BaseURL,
		})
	}
}

func (*Controller) decodeGoogleCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID            string
		Email         string
		VerifiedEmail bool
		Picture       string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", err
	}

	return userProfile.Email, nil
}

func (*Controller) decodeGithubCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Login     string
		AvatarURL string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", err
	}

	return userProfile.Login, nil
}

func (*Controller) decodeGitLabCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Username  string
		AvatarURL string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", nil
	}

	return userProfile.Username, nil
}

func (ctrl *Controller) newJWTToken(name string) (string, error) {
	claims := jwt.MapClaims{
		"iat":  time.Now().Unix(),
		"name": name,
	}

	if ctrl.config.LoginMaximumLifetimeDays > 0 {
		claims["exp"] = time.Now().Add(time.Hour * 24 * time.Duration(ctrl.config.LoginMaximumLifetimeDays)).Unix()
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return jwtToken.SignedString([]byte(ctrl.config.JWTSecret))
}

func (ctrl *Controller) logErrorAndRedirect(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if err != nil {
		ctrl.log.WithError(err).Error(msg)
	} else {
		ctrl.log.Error(msg)
	}
	invalidateCookie(w, stateCookieName)
	http.Redirect(w, r, "/forbidden", http.StatusTemporaryRedirect)
}

func (ctrl *Controller) callbackRedirectHandler(getAccountInfoURL string, info *oauthInfo, decodeResponse decodeResponseFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "missing state cookie", err)
			return
		}
		if cookie.Value != r.FormValue("state") {
			ctrl.logErrorAndRedirect(w, r, "invalid oauth state", nil)
			return
		}
		code := r.FormValue("code")
		if code == "" {
			ctrl.logErrorAndRedirect(w, r, "code not found", nil)
			return
		}

		callbackURL, err := getCallbackURL(r.Host, info.Config.RedirectURL, info.Type, r.URL.Query().Get("tls") == "true")
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "callbackURL parsing failed", nil)
			return
		}
		oauthConf := *info.Config
		oauthConf.RedirectURL = callbackURL
		token, err := oauthConf.Exchange(r.Context(), code)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "exchanging auth code for token failed", err)
			return
		}

		client := oauthConf.Client(r.Context(), token)
		resp, err := client.Get(getAccountInfoURL)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "failed to get oauth user info", err)
			return
		}
		defer resp.Body.Close()

		name, err := decodeResponse(resp)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "decoding response body failed", err)
			return
		}

		tk, err := ctrl.newJWTToken(name)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		createCookie(w, jwtCookieName, tk)

		tmpl, err := ctrl.getTemplate("/welcome.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render welcome page")
			return
		}

		mustExecute(tmpl, w, map[string]interface{}{
			"Name":    name,
			"BaseURL": ctrl.config.BaseURL,
		})
	}
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	fs := http.FileServer(ctrl.dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/comparison" || path == "/comparison-diff" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}

func (ctrl *Controller) getTemplate(path string) (*template.Template, error) {
	f, err := ctrl.dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %q", path, err)
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %q", path, err)
	}

	tmpl, err := template.New(path).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s template: %q", path, err)
	}
	return tmpl, nil
}

func (ctrl *Controller) renderIndexPage(w http.ResponseWriter, _ *http.Request) {
	tmpl, err := ctrl.getTemplate("/index.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render index page")
		return
	}

	initialStateObj := indexPageJSON{}
	ctrl.storage.GetValues("__name__", func(v string) bool {
		initialStateObj.AppNames = append(initialStateObj.AppNames, v)
		return true
	})

	var b []byte
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}

	initialStateStr := string(b)
	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = ioutil.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Errorf("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	w.Header().Add("Content-Type", "text/html")
	mustExecute(tmpl, w, map[string]string{
		"InitialState":      initialStateStr,
		"BuildInfo":         build.JSON(),
		"LatestVersionInfo": updates.LatestVersionJSON(),
		"ExtraMetadata":     extraMetadataStr,
		"BaseURL":           ctrl.config.BaseURL,
		"NotificationText":  ctrl.NotificationText(),
	})
}

func (ctrl *Controller) NotificationText() string {
	// TODO: implement backend support for alert text
	return ""
}

func mustExecute(t *template.Template, w io.Writer, v interface{}) {
	if err := t.Execute(w, v); err != nil {
		panic(err)
	}
}
