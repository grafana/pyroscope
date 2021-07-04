package server

import (
	"context"
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

const cookieName = "pyroscopeJWT"

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

	// drainable routes:
	routes := []route{
		{"/", ctrl.indexHandler()},
		{"/ingest", ctrl.ingestHandler},
		{"/render", ctrl.renderHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}

	addRoutes(mux, routes, ctrl.drainMiddleware, ctrl.authMiddleware)

	// auth routes:
	authRoutes := []route{
		{"/google/login", ctrl.googleLoginHandler()},
		{"/google/callback", ctrl.googleCallbackHandler()},
		{"/login", ctrl.loginHandler()},
		{"/logout", ctrl.logoutHandler()},
	}

	addRoutes(mux, authRoutes)

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
		jwtCookie, errCookie := r.Cookie(cookieName)
		if errCookie != nil {
			ctrl.httpServer.ErrorLog.Printf("There seems to be problem with jwt token cookie: %v", errCookie)
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
			return []byte(config.JWTSecret), nil
		})

		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Error parsing jwt token: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		if _, ok := token.Claims.(jwt.MapClaims); !ok || !token.Valid {
			ctrl.httpServer.ErrorLog.Printf("Token not valid")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		// TODO: Access logged in user information by replacing _ with claims above and uncommenting below
		// value, ok := claims.(jwt.MapClaims)["key"]
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

func (ctrl *Controller) googleCallbackHandler() http.HandlerFunc {
	type callbackResponse struct {
		ID            string
		Email         string
		VerifiedEmail bool
		Picture       string
	}

	return func(w http.ResponseWriter, r *http.Request) {
		oauthConf := &oauth2.Config{
			ClientID:     ctrl.config.GoogleClientID,
			ClientSecret: ctrl.config.GoogleClientSecret,
			RedirectURL:  ctrl.config.GoogleRedirectURL,
			Scopes:       strings.Split(ctrl.config.GoogleScopes, " "),
			Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GoogleAuthURL, TokenURL: ctrl.config.GoogleTokenURL},
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

		resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + url.QueryEscape(token.AccessToken))
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Failed to get oauth user info: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}
		defer resp.Body.Close()

		var userProfile callbackResponse
		err = json.NewDecoder(resp.Body).Decode(&userProfile)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Decoding response body failed: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			// TODO: Add if we want it to expire
			// "exp": time.Now().Add(144 * time.Hour).Unix(),
			// "iat": time.Now().Unix(),
			"email": userProfile.Email,
		})

		tk, err := jwtToken.SignedString([]byte(config.JWTSecret))
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Signing jwt failed: %v", err)
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		// TODO: Should user be logged out once google token expires?
		refreshCookie := &http.Cookie{
			Name:     cookieName,
			Path:     "/",
			Value:    tk,
			HttpOnly: true,
			MaxAge:   0,
			SameSite: http.SameSiteStrictMode,
		}

		http.SetCookie(w, refreshCookie)
		tmplt := template.New("welcome.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/welcome.html")
		params := map[string]string{"Email": userProfile.Email}

		tmplt.Execute(w, params)
		return
	}
}

func (ctrl *Controller) googleLoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		oauthConf := &oauth2.Config{
			ClientID:     ctrl.config.GoogleClientID,
			ClientSecret: ctrl.config.GoogleClientSecret,
			RedirectURL:  ctrl.config.GoogleRedirectURL,
			Scopes:       strings.Split(ctrl.config.GoogleScopes, " "),
			Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GoogleAuthURL, TokenURL: ctrl.config.GoogleTokenURL},
		}

		URL, err := url.Parse(oauthConf.Endpoint.AuthURL)
		if err != nil {
			ctrl.httpServer.ErrorLog.Printf("Parse: " + err.Error())
		}

		ctrl.httpServer.ErrorLog.Printf(URL.String())
		parameters := url.Values{}
		parameters.Add("client_id", oauthConf.ClientID)
		parameters.Add("scope", strings.Join(oauthConf.Scopes, " "))
		parameters.Add("redirect_uri", oauthConf.RedirectURL)
		parameters.Add("response_type", "code")
		URL.RawQuery = parameters.Encode()
		url := URL.String()
		ctrl.httpServer.ErrorLog.Printf(url)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func (ctrl *Controller) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./webapp/templates/login.html")
	}
}

func (ctrl *Controller) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refreshCookie := &http.Cookie{
			Name:     cookieName,
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
