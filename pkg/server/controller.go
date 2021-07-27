package server

import (
	"context"
	"errors"
	"fmt"
	golog "log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt"
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
	oauthGoogle     = iota
	oauthGithub
	oauthGitlab
)

type decodeResponseFunc func(*http.Response) (string, error)

type Controller struct {
	drained uint32

	config     *config.Server
	storage    *storage.Storage
	ingester   storage.Ingester
	log        *logrus.Logger
	httpServer *http.Server

	dir http.FileSystem

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus
}

func New(c *config.Server, s *storage.Storage, i storage.Ingester, l *logrus.Logger) (*Controller, error) {
	appStats, err := hyperloglog.NewPlus(uint8(18))
	if err != nil {
		return nil, err
	}

	ctrl := Controller{
		config:   c,
		log:      l,
		storage:  s,
		ingester: i,
		stats:    make(map[string]int),
		appStats: appStats,
	}

	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		ctrl.dir = pkger.Dir("/webapp/public")
	} else {
		ctrl.dir = http.Dir("./webapp/public")
	}

	return &ctrl, nil
}

func (ctrl *Controller) assetsFilesHandler(w http.ResponseWriter, r *http.Request) {
	fs := http.FileServer(ctrl.dir)
	fs.ServeHTTP(w, r)
}

func (ctrl *Controller) mux() (http.Handler, error) {
	mux := http.NewServeMux()

	// Routes not protected with auth. Drained at shutdown.
	insecureRoutes, err := ctrl.getAuthRoutes()
	if err != nil {
		return nil, err
	}
	insecureRoutes = append(insecureRoutes, []route{
		{"/ingest", ctrl.ingestHandler},
		{"/forbidden", ctrl.forbiddenHandler()},
		{"/assets/", ctrl.assetsFilesHandler},
	}...)
	addRoutes(mux, insecureRoutes, ctrl.drainMiddleware)

	// Protected routes:
	protectedRoutes := []route{
		{"/", ctrl.indexHandler()},
		{"/render", ctrl.renderHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}
	addRoutes(mux, protectedRoutes, ctrl.drainMiddleware, ctrl.authMiddleware)

	// Diagnostic secure routes: must be protected but not drained.
	diagnosticSecureRoutes := []route{
		{"/config", ctrl.configHandler},
		{"/build", ctrl.buildHandler},
	}
	if !ctrl.config.DisablePprofEndpoint {
		diagnosticSecureRoutes = append(diagnosticSecureRoutes, []route{
			{"/debug/pprof/", pprof.Index},
			{"/debug/pprof/cmdline", pprof.Cmdline},
			{"/debug/pprof/profile", pprof.Profile},
			{"/debug/pprof/symbol", pprof.Symbol},
			{"/debug/pprof/trace", pprof.Trace},
		}...)
	}
	addRoutes(mux, diagnosticSecureRoutes, ctrl.authMiddleware)
	addRoutes(mux, []route{
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/healthz", ctrl.healthz},
	})

	return mux, nil
}

type oauthInfo struct {
	Config  *oauth2.Config
	AuthURL *url.URL
	Type    int
}

func (ctrl *Controller) generateOauthInfo(oauthType int) *oauthInfo {
	switch oauthType {
	case oauthGoogle:
		googleOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GoogleClientID,
				ClientSecret: ctrl.config.GoogleClientSecret,
				Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GoogleAuthURL, TokenURL: ctrl.config.GoogleTokenURL},
			},
			Type: oauthGoogle,
		}
		if ctrl.config.GoogleRedirectURL != "" {
			googleOauthInfo.Config.RedirectURL = ctrl.config.GoogleRedirectURL
		}

		return googleOauthInfo
	case oauthGithub:
		githubOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GithubClientID,
				ClientSecret: ctrl.config.GithubClientSecret,
				Scopes:       []string{"read:user", "user:email"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GithubAuthURL, TokenURL: ctrl.config.GithubTokenURL},
			},
			Type: oauthGithub,
		}

		if ctrl.config.GithubRedirectURL != "" {
			githubOauthInfo.Config.RedirectURL = ctrl.config.GithubRedirectURL
		}

		return githubOauthInfo
	case oauthGitlab:
		gitlabOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GitlabApplicationID,
				ClientSecret: ctrl.config.GitlabClientSecret,
				Scopes:       []string{"read_user"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GitlabAuthURL, TokenURL: ctrl.config.GitlabTokenURL},
			},
			Type: oauthGitlab,
		}

		if ctrl.config.GitlabRedirectURL != "" {
			gitlabOauthInfo.Config.RedirectURL = ctrl.config.GitlabRedirectURL
		}

		return gitlabOauthInfo
	}

	return nil
}

func (ctrl *Controller) getAuthRoutes() ([]route, error) {
	authRoutes := []route{
		{"/login", ctrl.loginHandler()},
		{"/logout", ctrl.logoutHandler()},
	}

	if ctrl.config.GoogleEnabled {
		authURL, err := url.Parse(ctrl.config.GoogleAuthURL)
		if err != nil {
			return nil, err
		}

		googleOauthInfo := ctrl.generateOauthInfo(oauthGoogle)
		if googleOauthInfo != nil {
			googleOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/auth/google/login", ctrl.oauthLoginHandler(googleOauthInfo)},
				{"/auth/google/callback", ctrl.callbackHandler("/auth/google/redirect")},
				{"/auth/google/redirect", ctrl.callbackRedirectHandler(
					"https://www.googleapis.com/oauth2/v2/userinfo", googleOauthInfo, ctrl.decodeGoogleCallbackResponse)},
			}...)
		}
	}

	if ctrl.config.GithubEnabled {
		authURL, err := url.Parse(ctrl.config.GithubAuthURL)
		if err != nil {
			return nil, err
		}

		githubOauthInfo := ctrl.generateOauthInfo(oauthGithub)
		if githubOauthInfo != nil {
			githubOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/auth/github/login", ctrl.oauthLoginHandler(githubOauthInfo)},
				{"/auth/github/callback", ctrl.callbackHandler("/auth/github/redirect")},
				{"/auth/github/redirect", ctrl.callbackRedirectHandler("https://api.github.com/user", githubOauthInfo, ctrl.decodeGithubCallbackResponse)},
			}...)
		}
	}

	if ctrl.config.GitlabEnabled {
		authURL, err := url.Parse(ctrl.config.GitlabAuthURL)
		if err != nil {
			return nil, err
		}

		gitlabOauthInfo := ctrl.generateOauthInfo(oauthGitlab)
		if gitlabOauthInfo != nil {
			gitlabOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/auth/gitlab/login", ctrl.oauthLoginHandler(gitlabOauthInfo)},
				{"/auth/gitlab/callback", ctrl.callbackHandler("/auth/gitlab/redirect")},
				{"/auth/gitlab/redirect", ctrl.callbackRedirectHandler(ctrl.config.GitlabAPIURL, gitlabOauthInfo, ctrl.decodeGitLabCallbackResponse)},
			}...)
		}
	}

	return authRoutes, nil
}

func (ctrl *Controller) Start() error {
	logger := logrus.New()
	w := logger.Writer()
	defer w.Close()

	handler, err := ctrl.mux()
	if err != nil {
		return err
	}

	ctrl.httpServer = &http.Server{
		Addr:           ctrl.config.APIBindAddr,
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       golog.New(w, "", 0),
	}

	// ListenAndServe always returns a non-nil error. After Shutdown or Close,
	// the returned error is ErrServerClosed.
	err = ctrl.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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

func (ctrl *Controller) isAuthRequired() bool {
	return ctrl.config.GoogleEnabled || ctrl.config.GithubEnabled || ctrl.config.GitlabEnabled
}

func (ctrl *Controller) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.isAuthRequired() {
			next.ServeHTTP(w, r)
			return
		}

		jwtCookie, err := r.Cookie(jwtCookieName)
		if err != nil {
			ctrl.log.WithFields(logrus.Fields{
				"url":  r.URL.String(),
				"host": r.Header.Get("Host"),
			}).Debug("missing jwt cookie")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		_, err = jwt.Parse(jwtCookie.Value, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(ctrl.config.JWTSecret), nil
		})

		if err != nil {
			ctrl.log.WithError(err).Error("invalid jwt token")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (ctrl *Controller) writeInvalidParameterError(w http.ResponseWriter, err error) {
	ctrl.writeError(w, http.StatusBadRequest, err, "invalid parameter")
}

func (ctrl *Controller) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	ctrl.writeError(w, http.StatusInternalServerError, err, msg)
}

func (ctrl *Controller) writeJSONEncodeError(w http.ResponseWriter, err error) {
	ctrl.writeInternalServerError(w, err, "encoding response body")
}

func (ctrl *Controller) writeError(w http.ResponseWriter, code int, err error, msg string) {
	logrus.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (ctrl *Controller) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	logrus.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
