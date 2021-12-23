package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	golog "log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt"
	gmux "github.com/gorilla/mux"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"github.com/slok/go-http-metrics/middleware/std"

	adhocserver "github.com/pyroscope-io/pyroscope/pkg/adhoc/server"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
	"github.com/pyroscope-io/pyroscope/webapp"
)

const (
	jwtCookieName              = "pyroscopeJWT"
	stateCookieName            = "pyroscopeState"
	gzHTTPCompressionThreshold = 2000
	oauthGoogle                = iota
	oauthGithub
	oauthGitlab
)

type Controller struct {
	drained uint32

	config     *config.Server
	storage    *storage.Storage
	log        *logrus.Logger
	httpServer *http.Server
	notifier   Notifier
	metricsMdw middleware.Middleware
	dir        http.FileSystem

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus

	// Exported metrics.
	exportedMetrics *prometheus.Registry
	exporter        storage.MetricsExporter

	// Adhoc mode
	adhoc adhocserver.Server
}

type Config struct {
	Configuration *config.Server
	*logrus.Logger
	*storage.Storage
	Notifier

	// The registerer is used for exposing server metrics.
	MetricsRegisterer prometheus.Registerer

	// Exported metrics registry and exported.
	ExportedMetricsRegistry *prometheus.Registry
	storage.MetricsExporter

	Adhoc adhocserver.Server
}

type Notifier interface {
	// NotificationText returns message that will be displayed to user
	// on index page load. The message should point user to a critical problem.
	// TODO(kolesnikovae): we should poll for notifications (or subscribe).
	NotificationText() string
}

func New(c Config) (*Controller, error) {
	if c.Configuration.BaseURL != "" {
		_, err := url.Parse(c.Configuration.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("BaseURL is invalid: %w", err)
		}
	}

	ctrl := Controller{
		config:   c.Configuration,
		log:      c.Logger,
		storage:  c.Storage,
		exporter: c.MetricsExporter,
		notifier: c.Notifier,
		stats:    make(map[string]int),
		appStats: mustNewHLL(),

		exportedMetrics: c.ExportedMetricsRegistry,
		metricsMdw: middleware.New(middleware.Config{
			Recorder: metrics.NewRecorder(metrics.Config{
				Prefix:   "pyroscope",
				Registry: c.MetricsRegisterer,
			}),
		}),

		adhoc: c.Adhoc,
	}

	var err error
	ctrl.dir, err = webapp.Assets()
	if err != nil {
		return nil, err
	}

	return &ctrl, nil
}

func mustNewHLL() *hyperloglog.HyperLogLogPlus {
	hll, err := hyperloglog.NewPlus(uint8(18))
	if err != nil {
		panic(err)
	}
	return hll
}

func (ctrl *Controller) mux() (http.Handler, error) {
	r := gmux.NewRouter()

	// Routes not protected with auth. Drained at shutdown.
	insecureRoutes, err := ctrl.getAuthRoutes()
	if err != nil {
		return nil, err
	}

	ingestHandler := NewIngestHandler(ctrl.log, ctrl.storage, ctrl.exporter, func(pi *storage.PutInput) {
		ctrl.statsInc("ingest")
		ctrl.statsInc("ingest:" + pi.SpyName)
		ctrl.appStats.Add(hashString(pi.Key.AppName()))
	})

	insecureRoutes = append(insecureRoutes, []route{
		{"/ingest", ingestHandler.ServeHTTP},
		{"/forbidden", ctrl.forbiddenHandler()},
		{"/assets/", r.PathPrefix("/assets/").Handler(http.FileServer(ctrl.dir)).GetHandler().ServeHTTP},
	}...)
	ctrl.addRoutes(r, insecureRoutes, ctrl.drainMiddleware)

	// Protected routes:
	protectedRoutes := []route{
		{"/", ctrl.indexHandler()},
		{"/comparison", ctrl.indexHandler()},
		{"/comparison-diff", ctrl.indexHandler()},
		{"/adhoc-single", ctrl.indexHandler()},
		{"/adhoc-comparison", ctrl.indexHandler()},
		{"/adhoc-comparison-diff", ctrl.indexHandler()},
		{"/render", ctrl.renderHandler},
		{"/render-diff", ctrl.renderDiffHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
		{"/api/adhoc", ctrl.adhoc.AddRoutes(r.PathPrefix("/api/adhoc").Subrouter())},
	}
	ctrl.addRoutes(r, protectedRoutes, ctrl.drainMiddleware, ctrl.authMiddleware)

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

	ctrl.addRoutes(r, diagnosticSecureRoutes, ctrl.authMiddleware)
	ctrl.addRoutes(r, []route{
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/exported-metrics", ctrl.exportedMetricsHandler},
		{"/healthz", ctrl.healthz},
	})

	return r, nil
}

func (ctrl *Controller) exportedMetricsHandler(w http.ResponseWriter, r *http.Request) {
	promhttp.InstrumentMetricHandler(ctrl.exportedMetrics,
		promhttp.HandlerFor(ctrl.exportedMetrics, promhttp.HandlerOpts{})).
		ServeHTTP(w, r)
}

func (ctrl *Controller) getAuthRoutes() ([]route, error) {
	authRoutes := []route{
		{"/signup", ctrl.signupHandler},
		{"/login", ctrl.loginHandler},
		{"/logout", ctrl.logoutHandler},
	}

	if ctrl.config.Auth.Google.Enabled {
		googleHandler, err := newGoogleHandler(ctrl.config.Auth.Google, ctrl.config.BaseURL, ctrl.log)
		if err != nil {
			return nil, err
		}

		authRoutes = append(authRoutes, []route{
			{"/auth/google/login", ctrl.oauthLoginHandler(googleHandler)},
			{"/auth/google/callback", ctrl.callbackHandler(googleHandler.redirectRoute)},
			{"/auth/google/redirect", ctrl.callbackRedirectHandler(googleHandler)},
		}...)
	}

	if ctrl.config.Auth.Github.Enabled {
		githubHandler, err := newGithubHandler(ctrl.config.Auth.Github, ctrl.config.BaseURL, ctrl.log)
		if err != nil {
			return nil, err
		}

		authRoutes = append(authRoutes, []route{
			{"/auth/github/login", ctrl.oauthLoginHandler(githubHandler)},
			{"/auth/github/callback", ctrl.callbackHandler(githubHandler.redirectRoute)},
			{"/auth/github/redirect", ctrl.callbackRedirectHandler(githubHandler)},
		}...)
	}

	if ctrl.config.Auth.Gitlab.Enabled {
		gitlabHandler, err := newGitlabHandler(ctrl.config.Auth.Gitlab, ctrl.config.BaseURL, ctrl.log)
		if err != nil {
			return nil, err
		}

		authRoutes = append(authRoutes, []route{
			{"/auth/gitlab/login", ctrl.oauthLoginHandler(gitlabHandler)},
			{"/auth/gitlab/callback", ctrl.callbackHandler(gitlabHandler.redirectRoute)},
			{"/auth/gitlab/redirect", ctrl.callbackRedirectHandler(gitlabHandler)},
		}...)
	}

	return authRoutes, nil
}

func (ctrl *Controller) getHandler() (http.Handler, error) {
	handler, err := ctrl.mux()
	if err != nil {
		return nil, err
	}

	gzhttpMiddleware, err := gzhttp.NewWrapper(gzhttp.MinSize(gzHTTPCompressionThreshold), gzhttp.CompressionLevel(gzip.BestSpeed))
	if err != nil {
		return nil, err
	}

	return gzhttpMiddleware(handler), nil
}

func (ctrl *Controller) Start() error {
	logger := logrus.New()
	w := logger.Writer()
	defer w.Close()
	handler, err := ctrl.getHandler()
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

	updates.StartVersionUpdateLoop()

	if ctrl.config.TLSCertificateFile != "" && ctrl.config.TLSKeyFile != "" {
		err = ctrl.httpServer.ListenAndServeTLS(ctrl.config.TLSCertificateFile, ctrl.config.TLSKeyFile)
	} else {
		err = ctrl.httpServer.ListenAndServe()
	}

	// ListenAndServe always returns a non-nil error. After Shutdown or Close,
	// the returned error is ErrServerClosed.
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

func (ctrl *Controller) trackMetrics(route string) func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return std.Handler(route, ctrl.metricsMdw, next).ServeHTTP
	}
}

func (ctrl *Controller) isAuthRequired() bool {
	return ctrl.config.Auth.Google.Enabled || ctrl.config.Auth.Github.Enabled || ctrl.config.Auth.Gitlab.Enabled
}

func (ctrl *Controller) redirectPreservingBaseURL(w http.ResponseWriter, r *http.Request, urlStr string, status int) {
	if ctrl.config.BaseURL != "" {
		// we're modifying the URL here so I'm not memoizing it and instead parsing it all over again to create a new object
		u, err := url.Parse(ctrl.config.BaseURL)
		if err != nil {
			// TODO: technically this should never happen because NewController would return an error
			logrus.Error("base URL is invalid, some redirects might not work as expected")
		} else {
			u.Path = filepath.Join(u.Path, urlStr)
			urlStr = u.String()
		}
	}
	http.Redirect(w, r, urlStr, status)
}

func (ctrl *Controller) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.isAuthRequired() {
			next.ServeHTTP(w, r)
			return
		}

		// TODO(kolesnikovae): Replace with auth service.
		jwtCookie, err := r.Cookie(jwtCookieName)
		if err != nil {
			ctrl.log.WithFields(logrus.Fields{
				"url":  r.URL.String(),
				"host": r.Header.Get("Host"),
			}).Debug("missing jwt cookie")
			ctrl.redirectPreservingBaseURL(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		token, err := jwt.Parse(jwtCookie.Value, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(ctrl.config.Auth.JWTSecret), nil
		})

		if err != nil {
			ctrl.log.WithError(err).Error("invalid jwt token")
			ctrl.redirectPreservingBaseURL(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userName, ok := claims["name"]
			if !ok {
				ctrl.log.WithError(err).Error("missing user name token claim")
				ctrl.redirectPreservingBaseURL(w, r, "/login", http.StatusTemporaryRedirect)
			}
			// TODO(kolesnikovae): Inject user to the request.
			_ = userName
		}

		next.ServeHTTP(w, r)
	}
}

func (*Controller) expectFormats(format string) error {
	switch format {
	case "json", "pprof", "collapsed", "":
		return nil
	default:
		return errUnknownFormat
	}
}

func (ctrl *Controller) writeResponseJSON(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		ctrl.writeJSONEncodeError(w, err)
	}
}

func (*Controller) writeResponseFile(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func (ctrl *Controller) writeError(w http.ResponseWriter, code int, err error, msg string) {
	WriteError(ctrl.log, w, code, err, msg)
}

func (ctrl *Controller) writeInvalidMethodError(w http.ResponseWriter) {
	WriteErrorMessage(ctrl.log, w, http.StatusMethodNotAllowed, "method not allowed")
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

func (ctrl *Controller) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	WriteErrorMessage(ctrl.log, w, code, msg)
}

func WriteError(log *logrus.Logger, w http.ResponseWriter, code int, err error, msg string) {
	log.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func WriteErrorMessage(log *logrus.Logger, w http.ResponseWriter, code int, msg string) {
	log.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
