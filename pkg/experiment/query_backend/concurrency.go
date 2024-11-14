package query_backend

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/platinummonkey/go-concurrency-limits/core"
	gclGrpc "github.com/platinummonkey/go-concurrency-limits/grpc"
	"github.com/platinummonkey/go-concurrency-limits/limit"
	"github.com/platinummonkey/go-concurrency-limits/limiter"
	"github.com/platinummonkey/go-concurrency-limits/strategy"
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

func CreateConcurrencyInterceptor(logger log.Logger) (grpc.UnaryServerInterceptor, error) {
	gclLog := newGclLogger(logger)
	// TODO(aleks-p): Implement metric registry
	serverLimit, err := limit.NewGradient2Limit("query-backend-concurrency-limit", minLimit, maxLimit, initialLimit, queueSizeFn, smoothing, longWindow, gclLog, nil)
	if err != nil {
		return nil, err
	}

	serverLimiter, err := limiter.NewDefaultLimiter(serverLimit, minWindowTime, maxWindowTime, minRTTThreshold, windowSize, strategy.NewSimpleStrategy(initialLimit), gclLog, nil)
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
