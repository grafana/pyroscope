package util

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/felixge/httpsnoop"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/instrument"
	dslog "github.com/grafana/dskit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/tracing"
	"github.com/grafana/dskit/user"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/http2"
	"gopkg.in/yaml.v3"

	"github.com/grafana/pyroscope/pkg/tenant"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/util/nethttp"
)

var defaultTransport http.RoundTripper = &http2.Transport{
	AllowHTTP:        true,
	ReadIdleTimeout:  30 * time.Second,
	WriteByteTimeout: 30 * time.Second,
	PingTimeout:      90 * time.Second,
	DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr)
	},
}

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type RoundTripperInstrumentFunc func(next http.RoundTripper) http.RoundTripper

// InstrumentedDefaultHTTPClient returns an http client configured with some
// default settings which is wrapped with a variety of instrumented
// RoundTrippers.
func InstrumentedDefaultHTTPClient(instruments ...RoundTripperInstrumentFunc) *http.Client {
	client := &http.Client{
		Transport: defaultTransport,
	}
	return InstrumentedHTTPClient(client, instruments...)
}

// InstrumentedHTTPClient adds the associated instrumentation middlewares to the
// provided http client.
func InstrumentedHTTPClient(client *http.Client, instruments ...RoundTripperInstrumentFunc) *http.Client {
	for i := len(instruments) - 1; i >= 0; i-- {
		client.Transport = instruments[i](client.Transport)
	}
	return client
}

// WithTracingTransport wraps the given RoundTripper with a tracing instrumented
// one.
func WithTracingTransport() RoundTripperInstrumentFunc {
	return func(next http.RoundTripper) http.RoundTripper {
		next = &nethttp.Transport{RoundTripper: next}
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req = nethttp.TraceRequest(opentracing.GlobalTracer(), req)
			return next.RoundTrip(req)
		})
	}
}

// WriteYAMLResponse writes some YAML as a HTTP response.
func WriteYAMLResponse(w http.ResponseWriter, v interface{}) {
	// There is not standardised content-type for YAML, text/plain ensures the
	// YAML is displayed in the browser instead of offered as a download
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	data, err := yaml.Marshal(v)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// We ignore errors here, because we cannot do anything about them.
	// Write will trigger sending Status code, so we cannot send a different status code afterwards.
	// Also this isn't internal error, but error communicating with client.
	_, _ = w.Write(data)
}

const (
	maxResponseBodyInLogs = 4096 // At most 4k bytes from response bodies in our logs.
)

// Log middleware logs http requests
type Log struct {
	Log                   log.Logger
	LogRequestHeaders     bool // LogRequestHeaders true -> dump http headers at debug log level
	LogRequestAtInfoLevel bool // LogRequestAtInfoLevel true -> log requests at info log level
	SourceIPs             *middleware.SourceIPExtractor
}

// logWithRequest information from the request and context as fields.
func (l Log) logWithRequest(r *http.Request) log.Logger {
	localLog := l.Log
	traceID, ok := tracing.ExtractTraceID(r.Context())
	if ok {
		localLog = log.With(localLog, "traceID", traceID)
	}

	if l.SourceIPs != nil {
		ips := l.SourceIPs.Get(r)
		if ips != "" {
			localLog = log.With(localLog, "sourceIPs", ips)
		}
	}
	orgID := r.Header.Get(user.OrgIDHeaderName)
	if orgID == "" {
		localLog = user.LogWith(r.Context(), localLog)
	} else {
		localLog = log.With(localLog, "orgID", orgID)
	}
	return localLog
}

// measure request body size
type reqBody struct {
	b    io.ReadCloser
	read byteSize
}

func (w *reqBody) Read(p []byte) (int, error) {
	n, err := w.b.Read(p)
	if n > 0 {
		w.read += byteSize(n)
	}
	return n, err
}

func (w *reqBody) Close() error {
	return w.b.Close()
}

type byteSize uint64

func (bs byteSize) String() string {
	return strings.Replace(humanize.IBytes(uint64(bs)), " ", "", 1)
}

// Wrap implements Middleware
func (l Log) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		uri := r.RequestURI // capture the URI before running next, as it may get rewritten
		requestLog := l.logWithRequest(r)
		// Log headers before running 'next' in case other interceptors change the data.
		headers, err := dumpRequest(r)
		if err != nil {
			headers = nil
			level.Error(requestLog).Log("msg", "Could not dump request headers", "err", err)
		}
		var (
			httpErr       multierror.MultiError
			httpCode      int = http.StatusOK
			headerWritten bool
			buf           bytes.Buffer
			bodyLeft      int = maxResponseBodyInLogs
		)

		wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					next(code)
					if !headerWritten {
						httpCode = code
						headerWritten = true
					}
				}
			},

			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					n, err := next(p)
					headerWritten = true
					httpErr.Add(err)
					if httpCode >= 500 && httpCode < 600 {
						bodyLeft = captureResponseBody(p, bodyLeft, &buf)
					}
					return n, err
				}
			},

			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					n, err := next(src)
					headerWritten = true
					httpErr.Add(err)
					return n, err
				}
			},
		})

		origBody := r.Body
		defer func() {
			// No need to leak our Body wrapper beyond the scope of this handler.
			r.Body = origBody
		}()

		rBody := &reqBody{b: origBody}
		r.Body = rBody

		next.ServeHTTP(wrapped, r)

		statusCode, writeErr := httpCode, httpErr.Err()

		requestLogD := log.With(requestLog, "method", r.Method, "uri", uri, "status", statusCode, "duration", time.Since(begin))
		if rBody.read > 0 {
			requestLogD = log.With(requestLogD, "request_body_size", rBody.read)
		}

		if writeErr != nil {
			if errors.Is(writeErr, context.Canceled) {
				if l.LogRequestAtInfoLevel {
					level.Info(requestLogD).Log("msg", dslog.LazySprintf("request cancelled: %s ws: %v; %s", writeErr, IsWSHandshakeRequest(r), headers))
				} else {
					level.Debug(requestLogD).Log("msg", dslog.LazySprintf("request cancelled: %s ws: %v; %s", writeErr, IsWSHandshakeRequest(r), headers))
				}
			} else {
				level.Warn(requestLogD).Log("msg", dslog.LazySprintf("error: %s ws: %v; %s", writeErr, IsWSHandshakeRequest(r), headers))
			}

			return
		}
		if 100 <= statusCode && statusCode < 500 {
			if l.LogRequestAtInfoLevel {
				level.Info(requestLogD).Log("msg", "http request processed")
			} else {
				level.Debug(requestLogD).Log("msg", "http request processed")
			}
			if l.LogRequestHeaders && headers != nil {
				if l.LogRequestAtInfoLevel {
					level.Info(requestLog).Log("msg", dslog.LazySprintf("ws: %v; %s", IsWSHandshakeRequest(r), string(headers)))
				} else {
					level.Debug(requestLog).Log("msg", dslog.LazySprintf("ws: %v; %s", IsWSHandshakeRequest(r), string(headers)))
				}
			}
		} else {
			level.Warn(requestLog).Log("msg", dslog.LazySprintf("%s %s (%d) %s Response: %q ws: %v; %s",
				r.Method, uri, statusCode, time.Since(begin), buf.Bytes(), IsWSHandshakeRequest(r), headers))
		}
	})
}

func captureResponseBody(data []byte, bodyBytesLeft int, buf *bytes.Buffer) int {
	if bodyBytesLeft <= 0 {
		return 0
	}
	if len(data) > bodyBytesLeft {
		buf.Write(data[:bodyBytesLeft])
		_, _ = io.WriteString(buf, "...")
		return 0
	} else {
		buf.Write(data)
		return bodyBytesLeft - len(data)
	}
}

func dumpRequest(req *http.Request) ([]byte, error) {
	var b bytes.Buffer

	// Exclude some headers for security, or just that we don't need them when debugging
	err := req.Header.WriteSubset(&b, map[string]bool{
		"Cookie":        true,
		"X-Csrf-Token":  true,
		"Authorization": true,
	})
	if err != nil {
		return nil, err
	}

	ret := bytes.Replace(b.Bytes(), []byte("\r\n"), []byte("; "), -1)
	return ret, nil
}

// IsWSHandshakeRequest returns true if the given request is a websocket handshake request.
func IsWSHandshakeRequest(req *http.Request) bool {
	if strings.ToLower(req.Header.Get("Upgrade")) == "websocket" {
		// Connection header values can be of form "foo, bar, ..."
		parts := strings.Split(strings.ToLower(req.Header.Get("Connection")), ",")
		for _, part := range parts {
			if strings.TrimSpace(part) == "upgrade" {
				return true
			}
		}
	}
	return false
}

// NewHTTPMetricMiddleware creates a new middleware that automatically instruments HTTP requests from the given router.
func NewHTTPMetricMiddleware(mux *mux.Router, namespace string, reg prometheus.Registerer) (middleware.Interface, error) {
	// Prometheus histograms for requests.
	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "request_duration_seconds",
		Help:      "Time (in seconds) spent serving HTTP requests.",
		Buckets:   instrument.DefBuckets,
	}, []string{"method", "route", "status_code", "ws"})
	err := reg.Register(requestDuration)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			requestDuration = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	receivedMessageSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "request_message_bytes",
		Help:      "Size (in bytes) of messages received in the request.",
		Buckets:   middleware.BodySizeBuckets,
	}, []string{"method", "route"})
	err = reg.Register(receivedMessageSize)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			receivedMessageSize = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	sentMessageSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "response_message_bytes",
		Help:      "Size (in bytes) of messages sent in response.",
		Buckets:   middleware.BodySizeBuckets,
	}, []string{"method", "route"})

	err = reg.Register(sentMessageSize)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			sentMessageSize = already.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}

	inflightRequests := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "inflight_requests",
		Help:      "Current number of inflight requests.",
	}, []string{"method", "route"})
	err = reg.Register(inflightRequests)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			inflightRequests = already.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, err
		}
	}
	return middleware.Instrument{
		RouteMatcher:     mux,
		Duration:         requestDuration,
		RequestBodySize:  receivedMessageSize,
		ResponseBodySize: sentMessageSize,
		InflightRequests: inflightRequests,
	}, nil
}

// WriteHTMLResponse sends message as text/html response with 200 status code.
func WriteHTMLResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html")

	// Ignore inactionable errors.
	_, _ = w.Write([]byte(message))
}

// WriteTextResponse sends message as text/plain response with 200 status code.
func WriteTextResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/plain")

	// Ignore inactionable errors.
	_, _ = w.Write([]byte(message))
}

// RenderHTTPResponse either responds with JSON or a rendered HTML page using the passed in template
// by checking the Accepts header.
func RenderHTTPResponse(w http.ResponseWriter, v interface{}, t *template.Template, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		WriteJSONResponse(w, v)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := t.Execute(w, v)
	if err != nil {
		httputil.Error(w, err)
	}
}

// WriteJSONResponse writes some JSON as a HTTP response.
func WriteJSONResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")

	data, err := json.Marshal(v)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// We ignore errors here, because we cannot do anything about them.
	// Write will trigger sending Status code, so we cannot send a different status code afterwards.
	// Also this isn't internal error, but error communicating with client.
	_, _ = w.Write(data)
}

// AuthenticateUser propagates the user ID from HTTP headers back to the request's context.
// If on is false, it will inject the default tenant ID.
func AuthenticateUser(on bool) middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !on {
				next.ServeHTTP(w, r.WithContext(user.InjectOrgID(r.Context(), tenant.DefaultTenantID)))
				return
			}
			_, ctx, err := user.ExtractOrgIDFromHTTPRequest(r)
			if err != nil {
				httputil.ErrorWithStatus(w, err, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}
