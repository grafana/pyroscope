package querybackend

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/platinummonkey/go-concurrency-limits/core"
	gclGrpc "github.com/platinummonkey/go-concurrency-limits/grpc"
	"github.com/platinummonkey/go-concurrency-limits/limit"
	"github.com/platinummonkey/go-concurrency-limits/limiter"
	"github.com/platinummonkey/go-concurrency-limits/strategy"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const (
	// gradient2 limit
	minLimit     = 50
	maxLimit     = 100
	initialLimit = 50
	smoothing    = 0.2
	longWindow   = 600

	// limiter
	minWindowTime   = 1
	maxWindowTime   = 1000
	minRTTThreshold = 1e6
	windowSize      = 100
)

var (
	queueSizeFn = func(limit int) int {
		return 10
	}
)

func CreateConcurrencyInterceptor(logger log.Logger, reg prometheus.Registerer) (grpc.UnaryServerInterceptor, error) {
	gclLog := newGclLogger(logger)
	metricRegistry := newPrometheusMetricRegistry(reg)
	serverLimit, err := limit.NewGradient2Limit("query-backend", minLimit, maxLimit, initialLimit, queueSizeFn, smoothing, longWindow, gclLog, metricRegistry)
	if err != nil {
		return nil, err
	}

	serverLimiter, err := limiter.NewDefaultLimiter(serverLimit, minWindowTime, maxWindowTime, minRTTThreshold, windowSize, strategy.NewSimpleStrategy(initialLimit), gclLog, metricRegistry)
	if err != nil {
		return nil, err
	}

	options := []gclGrpc.InterceptorOption{
		gclGrpc.WithName("gcl-interceptor"),
		gclGrpc.WithLimiter(serverLimiter),
		gclGrpc.WithLimitExceededResponseClassifier(func(ctx context.Context, method string, req interface{}, l core.Limiter) (interface{}, codes.Code, error) {
			return nil, codes.ResourceExhausted, fmt.Errorf("concurrency limit exceeded")
		})}
	gclInterceptor := gclGrpc.UnaryServerInterceptor(options...)

	return gclInterceptor, err
}

type gclLogger struct {
	logger log.Logger
}

func (g gclLogger) Debugf(msg string, params ...interface{}) {
	level.Debug(g.logger).Log("msg", fmt.Sprintf(msg, params...))
}

func (g gclLogger) IsDebugEnabled() bool {
	return true
}

func newGclLogger(logger log.Logger) *gclLogger {
	return &gclLogger{logger: logger}
}

// prometheusMetricRegistry implements core.MetricRegistry backed by Prometheus.
type prometheusMetricRegistry struct {
	reg prometheus.Registerer
	mu  sync.Mutex

	histograms map[string]prometheus.Histogram
	counters   map[string]prometheus.Counter
}

func newPrometheusMetricRegistry(reg prometheus.Registerer) *prometheusMetricRegistry {
	return &prometheusMetricRegistry{
		reg:        reg,
		histograms: make(map[string]prometheus.Histogram),
		counters:   make(map[string]prometheus.Counter),
	}
}

const metricPrefix = "query_backend_concurrency_"
const gclNamePrefix = "query-backend."

// sanitizeID converts a gcl metric ID (e.g. "query-backend.rtt")
// to a valid Prometheus metric name (e.g. "query_backend_concurrency_rtt").
func sanitizeID(id string) string {
	suffix := strings.TrimPrefix(id, gclNamePrefix)
	return metricPrefix + strings.NewReplacer("-", "_", ".", "_").Replace(suffix)
}

func (r *prometheusMetricRegistry) getOrRegisterHistogram(id string) prometheus.Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[id]; ok {
		return h
	}
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: sanitizeID(id),
		Help: fmt.Sprintf("Concurrency limiter metric: %s", id),
	})
	if err := r.reg.Register(h); err != nil {
		// If already registered, use the existing collector.
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			h = are.ExistingCollector.(prometheus.Histogram)
		}
	}
	r.histograms[id] = h
	return h
}

func (r *prometheusMetricRegistry) getOrRegisterCounter(id string) prometheus.Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[id]; ok {
		return c
	}
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: sanitizeID(id),
		Help: fmt.Sprintf("Concurrency limiter metric: %s", id),
	})
	if err := r.reg.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			c = are.ExistingCollector.(prometheus.Counter)
		}
	}
	r.counters[id] = c
	return c
}

type histogramListener struct{ h prometheus.Histogram }

func (l *histogramListener) AddSample(value float64, tags ...string) { l.h.Observe(value) }

type counterListener struct{ c prometheus.Counter }

func (l *counterListener) AddSample(value float64, tags ...string) { l.c.Add(value) }

// RegisterDistribution registers a histogram for sample distributions.
func (r *prometheusMetricRegistry) RegisterDistribution(id string, tags ...string) core.MetricSampleListener {
	return &histogramListener{h: r.getOrRegisterHistogram(id)}
}

// RegisterTiming registers a histogram for timing samples (values in nanoseconds).
func (r *prometheusMetricRegistry) RegisterTiming(id string, tags ...string) core.MetricSampleListener {
	return &histogramListener{h: r.getOrRegisterHistogram(id)}
}

// RegisterCount registers a counter.
func (r *prometheusMetricRegistry) RegisterCount(id string, tags ...string) core.MetricSampleListener {
	return &counterListener{c: r.getOrRegisterCounter(id)}
}

// RegisterGauge registers a gauge backed by a supplier function.
func (r *prometheusMetricRegistry) RegisterGauge(id string, supplier core.MetricSupplier, tags ...string) {
	g := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: sanitizeID(id),
		Help: fmt.Sprintf("Concurrency limiter metric: %s", id),
	}, func() float64 {
		val, ok := supplier()
		if !ok {
			return 0
		}
		return val
	})
	if err := r.reg.Register(g); err != nil {
		// ignore if already registered
	}
}

// Start is a no-op for Prometheus (pull-based).
func (r *prometheusMetricRegistry) Start() {}

// Stop is a no-op for Prometheus (pull-based).
func (r *prometheusMetricRegistry) Stop() {}
