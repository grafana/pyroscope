package requestdump

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type Config struct {
	Enabled       bool    `yaml:"enabled"`
	SamplingRate  float64 `yaml:"sampling_rate"`
	MaxConcurrency int    `yaml:"max_concurrency"`
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "request-dump.enabled", false, "Enable dumping of HTTP requests to object storage")
	f.Float64Var(&c.SamplingRate, "request-dump.sampling-rate", 1.0, "Sampling rate for request dumping (0.0-1.0, where 1.0 means 100% of requests)")
	f.IntVar(&c.MaxConcurrency, "request-dump.max-concurrency", 2, "Maximum number of concurrent request dumps")
}

type Middleware struct {
	cfg          Config
	logger       log.Logger
	bucket       objstore.Bucket
	counter      uint64
	inflightChan chan struct{}
}

func NewMiddleware(cfg Config, logger log.Logger, bucket objstore.Bucket) middleware.Interface {
	return &Middleware{
		cfg:          cfg,
		logger:       logger,
		bucket:       bucket,
		inflightChan: make(chan struct{}, cfg.MaxConcurrency),
	}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if !m.cfg.Enabled || m.bucket == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.cfg.SamplingRate < 1.0 && rand.Float64() > m.cfg.SamplingRate {
			next.ServeHTTP(w, r)
			return
		}
		if !writeRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				level.Warn(m.logger).Log("msg", "failed to read request body", "err", err, "url", r.URL.String())
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		tenantID, err := tenant.ExtractTenantIDFromContext(r.Context())
		if err != nil {
			tenantID = "unknown"
		}

		select {
		case m.inflightChan <- struct{}{}:
			reqCopy := r.Clone(context.Background())
			if len(bodyBytes) > 0 {
				reqCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			go m.dumpFullRequest(reqCopy, tenantID)
		default:
		}

		next.ServeHTTP(w, r)
	})
}

func writeRequest(r *http.Request) bool {
	return r.URL.Path == "/ingest" ||
		r.URL.Path == "/push.v1.PusherService/Push" ||
		r.URL.Path == "/opentelemetry.proto.collector.profiles.v1development.ProfilesService/Export"
}

func (m *Middleware) dumpFullRequest(r *http.Request, tenantID string) {
	defer func() {
		<-m.inflightChan
	}()

	requestNum := atomic.AddUint64(&m.counter, 1)
	timestamp := time.Now().UTC().Format("2006-01-02_15:04:05.000000000")

	sanitizedPath := sanitizePath(r.URL.Path)
	filename := fmt.Sprintf("%s_%d_%s.bin", timestamp, requestNum, sanitizedPath)

	var buf bytes.Buffer
	if err := r.Write(&buf); err != nil {
		level.Warn(m.logger).Log("msg", "failed to serialize request", "err", err, "path", filename, "tenant", tenantID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prefixedBucket := objstore.NewPrefixedBucket(m.bucket, fmt.Sprintf("request-dump/%s/", tenantID))
	defer prefixedBucket.Close()

	if err := prefixedBucket.Upload(ctx, filename, bytes.NewReader(buf.Bytes())); err != nil {
		level.Warn(m.logger).Log("msg", "failed to upload request dump", "path", filename, "err", err, "tenant", tenantID)
	}
}

var sanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

func sanitizePath(path string) string {
	parsedURL, err := url.Parse(path)
	if err != nil {
		path = "invalid"
	} else {
		path = parsedURL.Path
	}

	if path == "" || path == "/" {
		return "root"
	}

	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	sanitized := sanitizeRegex.ReplaceAllString(path, "_")

	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}

	return sanitized
}