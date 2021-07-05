package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	golog "log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/markbates/pkger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
)

const (
	jwtCookieName   = "pyroscopeJWT"
	stateCookieName = "pyroscopeState"
)

type Controller struct {
	drained uint32

	config     *config.Server
	storage    *storage.Storage
	httpServer *http.Server

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus
}

func New(c *config.Server, s *storage.Storage) (*Controller, error) {
	appStats, err := hyperloglog.NewPlus(uint8(18))
	if err != nil {
		return nil, err
	}

	ctrl := Controller{
		config:   c,
		storage:  s,
		stats:    make(map[string]int),
		appStats: appStats,
	}

	return &ctrl, nil
}

func (ctrl *Controller) mux() http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, []route{
		{"/healthz", ctrl.healthz},
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/config", ctrl.configHandler},
		{"/build", ctrl.buildHandler},
	})

	// auth routes
	addRoutes(mux, ctrl.getAuthRoutes())

	// drainable routes:
	routes := []route{
		{"/", ctrl.indexHandler()},
		{"/ingest", ctrl.ingestHandler},
		{"/render", ctrl.renderHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}

	addRoutes(mux, routes, ctrl.drainMiddleware, ctrl.authMiddleware)

	if !ctrl.config.DisablePprofEndpoint {
		addRoutes(mux, []route{
			{"/debug/pprof/", pprof.Index},
			{"/debug/pprof/cmdline", pprof.Cmdline},
			{"/debug/pprof/profile", pprof.Profile},
			{"/debug/pprof/symbol", pprof.Symbol},
			{"/debug/pprof/trace", pprof.Trace},
		})
	}
	return mux
}

func getNewRedirectURL(url string) string {
	splitRedirect := strings.Split(url, "/")
	splitRedirect[len(splitRedirect)-1] = "redirect"
	return strings.Join(splitRedirect, "/")
}

func (ctrl *Controller) getAuthRoutes() []route {
	authRoutes := []route{
		{"/login", ctrl.loginHandler()},
		{"/logout", ctrl.logoutHandler()},
	}

	if ctrl.config.GoogleEnabled {
		googleOauthConfig := &oauth2.Config{
			ClientID:     ctrl.config.GoogleClientID,
			ClientSecret: ctrl.config.GoogleClientSecret,
			RedirectURL:  ctrl.config.GoogleRedirectURL,
			Scopes:       strings.Split(ctrl.config.GoogleScopes, " "),
			Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GoogleAuthURL, TokenURL: ctrl.config.GoogleTokenURL},
		}

		authRoutes = append(authRoutes, []route{
			{"/google/login", ctrl.oauthLoginHandler(googleOauthConfig)},
			{"/google/callback", ctrl.callbackHandler(getNewRedirectURL(ctrl.config.GoogleRedirectURL))},
			{"/google/redirect", ctrl.callbacRedirectkHandler(
				"https://www.googleapis.com/oauth2/v2/userinfo", googleOauthConfig, ctrl.decodeGoogleCallbackResponse)},
		}...)
	}

	if ctrl.config.GithubEnabled {
		gitHubOauthConfig := &oauth2.Config{
			ClientID:     ctrl.config.GithubClientID,
			ClientSecret: ctrl.config.GithubClientSecret,
			RedirectURL:  ctrl.config.GithubRedirectURL,
			Scopes:       strings.Split(ctrl.config.GithubScopes, " "),
			Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GithubAuthURL, TokenURL: ctrl.config.GithubTokenURL},
		}

		authRoutes = append(authRoutes, []route{
			{"/github/login", ctrl.oauthLoginHandler(gitHubOauthConfig)},
			{"/github/callback", ctrl.callbackHandler(getNewRedirectURL(ctrl.config.GithubRedirectURL))},
			{"/github/redirect", ctrl.callbacRedirectkHandler("https://api.github.com/user", gitHubOauthConfig, ctrl.decodeGithubCallbackResponse)},
		}...)
	}

	if ctrl.config.GitlabEnabled {
		gitLabOauthConfig := &oauth2.Config{
			ClientID:     ctrl.config.GitlabApplicationID,
			ClientSecret: ctrl.config.GitlabClientSecret,
			RedirectURL:  ctrl.config.GitlabRedirectURL,
			Scopes:       strings.Split(ctrl.config.GitlabScopes, " "),
			Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GitlabAuthURL, TokenURL: ctrl.config.GitlabTokenURL},
		}

		authRoutes = append(authRoutes, []route{
			{"/gitlab/login", ctrl.oauthLoginHandler(gitLabOauthConfig)},
			{"/gitlab/callback", ctrl.callbackHandler(getNewRedirectURL(ctrl.config.GitlabRedirectURL))},
			{"/gitlab/redirect", ctrl.callbacRedirectkHandler(ctrl.config.GitlabAPIURL, gitLabOauthConfig, ctrl.decodeGitLabCallbackResponse)},
		}...)
	}

	return authRoutes
}

func (ctrl *Controller) Start() error {
	logger := logrus.New()
	w := logger.Writer()
	defer w.Close()

	ctrl.httpServer = &http.Server{
		Addr:           ctrl.config.APIBindAddr,
		Handler:        ctrl.mux(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       golog.New(w, "", 0),
	}

	// ListenAndServe always returns a non-nil error. After Shutdown or Close,
	// the returned error is ErrServerClosed.
	err := ctrl.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("listen and serve: %v", err)
}

func (ctrl *Controller) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return ctrl.httpServer.Shutdown(ctx)
}

func (ctrl *Controller) Drain() {
	atomic.StoreUint32(&ctrl.drained, 1)
}

func (ctrl *Controller) drainMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ctrl.drained) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (ctrl *Controller) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwtCookie, err := r.Cookie(jwtCookieName)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("There seems to be problem with jwt token cookie: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		if jwtCookie == nil {
			ctrl.httpServer.ErrorLog.Printf("Missing jwt cookie")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		jwtToken := jwtCookie.Value
		token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(ctrl.config.JWTSecret), nil
		})

		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Error parsing jwt token: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			ctrl.httpServer.ErrorLog.Printf("Token not valid")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		if exp, ok := claims["exp"].(float64); ok && int64(exp) < time.Now().Unix() {
			ctrl.httpServer.ErrorLog.Printf("Token no longer valid")

			refreshCookie := &http.Cookie{
				Name: jwtCookieName,

				Path:     "/",
				Value:    "",
				HttpOnly: true,
				// MaxAge -1 request cookie be deleted immediately
				MaxAge:   -1,
				SameSite: http.SameSiteStrictMode,
			}

			http.SetCookie(w, refreshCookie)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	})
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

func (ctrl *Controller) decodeGoogleCallbackResponse(resp *http.Response) (name string, err error) {
	type callbackResponse struct {
		ID            string
		Email         string
		VerifiedEmail bool
		Picture       string
	}

	var userProfile callbackResponse
	err = json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return
	}

	name = userProfile.Email
	return
}

func (ctrl *Controller) decodeGithubCallbackResponse(resp *http.Response) (name string, err error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Login     string
		AvatarURL string
	}

	var userProfile callbackResponse
	err = json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return
	}

	name = userProfile.Login
	return
}

func (ctrl *Controller) decodeGitLabCallbackResponse(resp *http.Response) (name string, err error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Username  string
		AvatarURL string
	}

	var userProfile callbackResponse
	err = json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return
	}

	name = userProfile.Username
	return
}

func (ctrl *Controller) callbacRedirectkHandler(getAccountInfoURL string, oauthConf *oauth2.Config, decodeResponse func(*http.Response) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("There seems to be problem with state cookie: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		if cookie == nil {
			ctrl.httpServer.ErrorLog.Printf("Missing state cookie")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		cookieState := cookie.Value

		state := r.FormValue("state")
		if state != cookieState {
			ctrl.httpServer.ErrorLog.Printf("invalid oauth state, expected %v got %v", cookieState, state)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			ctrl.httpServer.ErrorLog.Printf("Code not found")
			w.Write([]byte("Code Not Found to provide AccessToken..\n"))
			reason := r.FormValue("error_reason")
			if reason == "user_denied" {
				w.Write([]byte("User has denied Permission.."))
			}

			return
		}

		token, err := oauthConf.Exchange(oauth2.NoContext, code)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Exchanging auth code for token failed with %v ", err)
			return
		}

		client := oauthConf.Client(oauth2.NoContext, token)
		resp, err := client.Get(getAccountInfoURL)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Failed to get oauth user info: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}
		defer resp.Body.Close()

		name, err := decodeResponse(resp)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Decoding response body failed: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

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
			ctrl.httpServer.ErrorLog.Printf("Signing jwt failed: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		// delete state cookie and add refresh cookie
		stateCookie := &http.Cookie{
			Name:     stateCookieName,
			Path:     "/",
			Value:    "",
			HttpOnly: true,
			// MaxAge -1 request cookie be deleted immediately
			MaxAge:   -1,
			SameSite: http.SameSiteStrictMode,
		}

		http.SetCookie(w, stateCookie)

		refreshCookie := &http.Cookie{
			Name:     jwtCookieName,
			Path:     "/",
			Value:    tk,
			HttpOnly: true,
			MaxAge:   0,
			SameSite: http.SameSiteStrictMode,
		}

		http.SetCookie(w, refreshCookie)
		tmplt := template.New("welcome.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/welcome.html")
		params := map[string]string{"Name": name}

		tmplt.Execute(w, params)
		return
	}
}

func generateStateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (ctrl *Controller) oauthLoginHandler(oauthConf *oauth2.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		URL, err := url.Parse(oauthConf.Endpoint.AuthURL)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Parse error: %v", err)
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
			ctrl.httpServer.ErrorLog.Printf("Generate token error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		parameters.Add("state", state)
		URL.RawQuery = parameters.Encode()
		url := URL.String()

		stateCookie := &http.Cookie{
			Name:     stateCookieName,
			Path:     "/",
			Value:    state,
			HttpOnly: true,
			MaxAge:   0,
			SameSite: http.SameSiteStrictMode,
		}
		http.SetCookie(w, stateCookie)

		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

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

func (ctrl *Controller) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refreshCookie := &http.Cookie{
			Name: jwtCookieName,

			Path:     "/",
			Value:    "",
			HttpOnly: true,
			// MaxAge -1 request cookie be deleted immediately
			MaxAge:   -1,
			SameSite: http.SameSiteStrictMode,
		}

		http.SetCookie(w, refreshCookie)
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	}
}

// Instead of this handler that just redirects, Javascript code can be added to load the state and send it to backend
// this is done so that the state cookie would be send back from browser
func (ctrl *Controller) callbackHandler(callbackURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parsedUrl, err := url.Parse(callbackURL)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Parse error: %v", err)
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
