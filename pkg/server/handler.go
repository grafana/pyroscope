package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/markbates/pkger"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

func (ctrl *Controller) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmplt := template.New("login.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/login.html")
		params := map[string]bool{
			"GoogleEnabled": ctrl.config.GoogleEnabled,
			"GithubEnabled": ctrl.config.GithubEnabled,
			"GitlabEnabled": ctrl.config.GitlabEnabled,
		}

		tmplt.Execute(w, params)
	}
}

func createCookie(w http.ResponseWriter, name, value string) {
	cookie := &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

func invalidateCookie(w http.ResponseWriter, name string) {
	cookie := &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie(w, cookie)
}

func (ctrl *Controller) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		invalidateCookie(w, jwtCookieName)
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
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

func (ctrl *Controller) oauthLoginHandler(oauthConf *oauth2.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authURL, err := url.Parse(oauthConf.Endpoint.AuthURL)
		if err != nil {
			ctrl.log.Errorf("Parse error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		parameters := url.Values{}
		parameters.Add("client_id", oauthConf.ClientID)
		parameters.Add("scope", strings.Join(oauthConf.Scopes, " "))
		parameters.Add("redirect_uri", oauthConf.RedirectURL)
		parameters.Add("response_type", "code")

		// generate state token for CSRF protection
		state, err := generateStateToken(16)
		if err != nil {
			ctrl.log.Errorf("Generate token error: %v", err)
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
func (ctrl *Controller) callbackHandler(callbackURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parsedUrl, err := url.Parse(callbackURL)
		if err != nil {
			ctrl.log.Errorf("Parse error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		parsedUrl.RawQuery = r.URL.Query().Encode()
		tmplt := template.New("redirect.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/redirect.html")
		params := map[string]string{"CallbackURL": parsedUrl.String()}

		tmplt.Execute(w, params)
	}
}

func (ctrl *Controller) decodeGoogleCallbackResponse(resp *http.Response) (string, error) {
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

func (ctrl *Controller) decodeGithubCallbackResponse(resp *http.Response) (string, error) {
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

func (ctrl *Controller) decodeGitLabCallbackResponse(resp *http.Response) (string, error) {
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
	tk, err := jwtToken.SignedString([]byte(ctrl.config.JWTSecret))
	if err != nil {
		return "", err
	}

	return tk, nil
}

func (ctrl *Controller) callbackRedirectHandler(getAccountInfoURL string, oauthConf *oauth2.Config, decodeResponse decodeResponseFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			ctrl.log.Error("Missing state cookie")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		cookieState := cookie.Value
		requestState := r.FormValue("state")

		if requestState != cookieState {
			ctrl.log.Error("invalid oauth state")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			ctrl.log.Error("Code not found")
			invalidateCookie(w, stateCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		token, err := oauthConf.Exchange(r.Context(), code)
		if err != nil {
			ctrl.log.Errorf("Exchanging auth code for token failed with %v ", err)
			invalidateCookie(w, stateCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		client := oauthConf.Client(r.Context(), token)
		resp, err := client.Get(getAccountInfoURL)
		if err != nil {
			ctrl.log.Errorf("Failed to get oauth user info: %v", err)
			invalidateCookie(w, stateCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}
		defer resp.Body.Close()

		name, err := decodeResponse(resp)
		if err != nil {
			ctrl.log.Errorf("Decoding response body failed: %v", err)
			invalidateCookie(w, stateCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		tk, err := ctrl.newJWTToken(name)
		if err != nil {
			ctrl.log.Errorf("Signing jwt failed: %v", err)
			invalidateCookie(w, stateCookieName)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		}

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		createCookie(w, jwtCookieName, tk)

		tmplt := template.New("welcome.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/welcome.html")
		params := map[string]string{"Name": name}

		tmplt.Execute(w, params)
		return
	}
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	var dir http.FileSystem
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		dir = pkger.Dir("/webapp/public")
	} else {
		dir = http.Dir("./webapp/public")
	}
	fs := http.FileServer(dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else if r.URL.Path == "/comparison" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

func renderServerError(rw http.ResponseWriter, text string) {
	rw.WriteHeader(500)
	rw.Write([]byte(text))
	rw.Write([]byte("\n"))
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}
type indexPage struct {
	InitialState  string
	BuildInfo     string
	ExtraMetadata string
	BaseURL       string
}

func (ctrl *Controller) renderIndexPage(dir http.FileSystem, rw http.ResponseWriter, _ *http.Request) {
	f, err := dir.Open("/index.html")
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not find file index.html: %q", err))
		return
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not read file index.html: %q", err))
		return
	}

	tmpl, err := template.New("index.html").Parse(string(b))
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not parse index.html template: %q", err))
		return
	}

	initialStateObj := indexPageJSON{}
	ctrl.storage.GetValues("__name__", func(v string) bool {
		initialStateObj.AppNames = append(initialStateObj.AppNames, v)
		return true
	})
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal initialStateObj json: %q", err))
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

	rw.Header().Add("Content-Type", "text/html")
	err = tmpl.Execute(rw, indexPage{
		InitialState:  initialStateStr,
		BuildInfo:     build.JSON(),
		ExtraMetadata: extraMetadataStr,
		BaseURL:       ctrl.config.BaseURL,
	})
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal json: %q", err))
		return
	}
}
