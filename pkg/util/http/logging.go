package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/felixge/httpsnoop"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/tracing"
	"github.com/grafana/dskit/user"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxResponseBodyInLogs = 4096 // At most 4k bytes from response bodies in our logs.
)

var timeNow = time.Now

// Log middleware logs http requests
type Log struct {
	Log                      log.Logger
	LogRequestHeaders        bool
	LogRequestExcludeHeaders []string
	LogRequestAtInfoLevel    bool // LogRequestAtInfoLevel true -> log requests at info log level
	SourceIPs                *middleware.SourceIPExtractor

	filterHeaderMap  map[string]struct{}
	filterHeaderOnce sync.Once
}

func (l *Log) filterHeader(key string) bool {
	// ensure map is populated once
	l.filterHeaderOnce.Do(func() {
		l.filterHeaderMap = make(map[string]struct{})
		for _, k := range l.LogRequestExcludeHeaders {
			l.filterHeaderMap[textproto.CanonicalMIMEHeaderKey(k)] = struct{}{}
		}
		for k := range middleware.AlwaysExcludedHeaders {
			l.filterHeaderMap[textproto.CanonicalMIMEHeaderKey(k)] = struct{}{}
		}
	})
	_, filter := l.filterHeaderMap[key]
	return filter
}

func (l *Log) extractHeaders(req *http.Request) []any {
	// Populate header list first and sort it
	logKeys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		if l.filterHeader(k) {
			continue
		}
		logKeys = append(logKeys, k)
	}
	slices.SortFunc(logKeys, strings.Compare)

	// build the log fields
	logFields := make([]any, 0, len(logKeys)*2)
	for _, k := range logKeys {
		logFields = append(
			logFields,
			"request_header_"+k,
			req.Header.Get(k),
		)
	}

	return logFields
}

// logWithRequest information from the request and context as fields.
func (l *Log) logWithRequest(r *http.Request) log.Logger {
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

	tenantID := r.Header.Get(user.OrgIDHeaderName)
	if tenantID == "" {
		id, err := user.ExtractOrgID(r.Context())
		if err == nil {
			tenantID = id
		}
	}
	if tenantID != "" {
		localLog = log.With(localLog, "tenant", tenantID)
	}

	return localLog
}

// measure request body size
type reqBody struct {
	b    io.ReadCloser
	read byteSize

	start    time.Time
	duration time.Duration

	sp trace.Span
}

func (w *reqBody) Read(p []byte) (int, error) {
	if w.start.IsZero() {
		w.start = timeNow()
		if w.sp != nil {
			w.sp.AddEvent("start reading body from request")
		}
	}
	n, err := w.b.Read(p)
	if n > 0 {
		w.read += byteSize(n)
	}
	if err == io.EOF {
		w.duration = timeNow().Sub(w.start)
		if w.sp != nil {
			w.sp.AddEvent("read body from request")
			if w.read > 0 {
				w.sp.SetAttributes(attribute.Int64("request_body_size", int64(w.read)))
			}
		}
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
func (l *Log) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := timeNow()
		uri := r.RequestURI // Capture the URI before running next, as it may get rewritten
		requestLog := l.logWithRequest(r)
		// Log headers before running 'next' in case other interceptors change the data.

		var (
			httpErr       multierror.MultiError
			httpCode      = http.StatusOK
			headerWritten bool
			buf           bytes.Buffer
			bodyLeft      = maxResponseBodyInLogs
		)

		headerFields := l.extractHeaders(r)

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
					if httpCode >= 400 && httpCode < 600 {
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

		rBody := &reqBody{
			b:  origBody,
			sp: trace.SpanFromContext(r.Context()),
		}
		r.Body = rBody

		next.ServeHTTP(wrapped, r)

		statusCode, writeErr := httpCode, httpErr.Err()

		requestLog = log.With(requestLog, "method", r.Method, "uri", uri, "status", statusCode, "duration", time.Since(begin), "proto", r.Proto)

		if l.LogRequestHeaders {
			requestLog = log.With(requestLog, headerFields...)
		}
		if rBody.read > 0 {
			requestLog = log.With(requestLog, "request_body_size", rBody.read)
			if rBody.duration > 0 {
				requestLog = log.With(requestLog, "request_body_read_duration", rBody.duration)
			}
		}

		requestLvl := level.Debug
		if l.LogRequestAtInfoLevel {
			requestLvl = level.Info
		}

		// log successful requests
		if writeErr == nil && (100 <= statusCode && statusCode < 400) {
			requestLvl(requestLog).Log("msg", "http request processed")
			return
		}

		// context cancelled is not considered a failure
		if writeErr != nil && errors.Is(writeErr, context.Canceled) {
			requestLvl(requestLog).Log("msg", "request cancelled")
			return
		}

		// add request headers if not anyhow added
		if !l.LogRequestHeaders {
			requestLog = log.With(requestLog, headerFields...)
		}

		// writeError shouldn't log the body
		if writeErr != nil {
			level.Warn(requestLog).Log("msg", "http request failed", "err", writeErr)
			return
		}

		level.Warn(requestLog).Log("msg", "http request failed", "response_body", buf.Bytes())
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
	}

	buf.Write(data)
	return bodyBytesLeft - len(data)
}
