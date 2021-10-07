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
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"github.com/slok/go-http-metrics/middleware/std"
	"github.com/valyala/bytebufferpool"

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

	config          *config.Server
	storage         *storage.Storage
	ingester        storage.Ingester
	log             *logrus.Logger
	httpServer      *http.Server
	exportedMetrics *prometheus.Registry
	metricsMdw      middleware.Middleware

	dir http.FileSystem

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus

	// Byte buffers are used for deserialization of ingested data.
	bufferPool bytebufferpool.Pool
}

type Config struct {
	Configuration *config.Server
	Storage       *storage.Storage
	Ingester      storage.Ingester
	Logger        *logrus.Logger

	MetricsRegisterer       prometheus.Registerer
	ExportedMetricsRegistry *prometheus.Registry
}

func New(c Config) (*Controller, error) {
	ctrl := Controller{
		config:   c.Configuration,
		log:      c.Logger,
		storage:  c.Storage,
		ingester: c.Ingester,
		stats:    make(map[string]int),
		appStats: mustNewHLL(),

		exportedMetrics: c.ExportedMetricsRegistry,
		metricsMdw: middleware.New(middleware.Config{
			Recorder: metrics.NewRecorder(metrics.Config{
				Prefix:   "pyroscope",
				Registry: c.MetricsRegisterer,
			}),
		}),
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
	ctrl.addRoutes(mux, insecureRoutes, ctrl.drainMiddleware)

	// Protected routes:
	protectedRoutes := []route{
		{"/", ctrl.indexHandler()},
		{"/render", ctrl.renderHandler},
		{"/render-diff", ctrl.renderDiffHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}
	ctrl.addRoutes(mux, protectedRoutes, ctrl.drainMiddleware, ctrl.authMiddleware)

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

	ctrl.addRoutes(mux, diagnosticSecureRoutes, ctrl.authMiddleware)
	ctrl.addRoutes(mux, []route{
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/exported-metrics", ctrl.exportedMetricsHandler},
		{"/healthz", ctrl.healthz},
	})

	return mux, nil
}

func (ctrl *Controller) exportedMetricsHandler(w http.ResponseWriter, r *http.Request) {
	promhttp.InstrumentMetricHandler(ctrl.exportedMetrics,
		promhttp.HandlerFor(ctrl.exportedMetrics, promhttp.HandlerOpts{})).
		ServeHTTP(w, r)
}

func (ctrl *Controller) getAuthRoutes() ([]route, error) {
	authRoutes := []route{
		{"/login", ctrl.loginHandler()},
		{"/logout", ctrl.logoutHandler()},
	}

	if ctrl.config.Auth.Google.Enabled {
		googleHandler, err := newGoogleHandler(ctrl.config.Auth.Google, ctrl.log)
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
		githubHandler, err := newGithubHandler(ctrl.config.Auth.Github, ctrl.log)
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
		gitlabHandler, err := newGitlabHandler(ctrl.config.Auth.Gitlab, ctrl.log)
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
			return []byte(ctrl.config.Auth.JWTSecret), nil
		})

		if err != nil {
			ctrl.log.WithError(err).Error("invalid jwt token")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (*Controller) expectJSON(format string) error {
	switch format {
	case "json", "":
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

func (ctrl *Controller) writeInvalidMethodError(w http.ResponseWriter, err error) {
	ctrl.writeError(w, http.StatusMethodNotAllowed, err, "method not supported")
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
	ctrl.log.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (ctrl *Controller) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	ctrl.log.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
