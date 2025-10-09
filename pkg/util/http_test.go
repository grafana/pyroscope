package util

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/tenant"
)

func TestWriteTextResponse(t *testing.T) {
	w := httptest.NewRecorder()

	WriteTextResponse(w, "hello world")

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "hello world", w.Body.String())
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
}

func TestMultitenantMiddleware(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://localhost:8080", nil)

	// No org ID header.
	m := AuthenticateUser(true).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := tenant.ExtractTenantIDFromContext(r.Context())
		require.NoError(t, err)
		assert.Equal(t, "1", id)
	}))
	m.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	r.Header.Set("X-Scope-OrgID", "1")
	m.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	// No org ID header without auth.
	r = httptest.NewRequest("GET", "http://localhost:8080", nil)
	m = AuthenticateUser(false).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := tenant.ExtractTenantIDFromContext(r.Context())
		require.NoError(t, err)
		assert.Equal(t, tenant.DefaultTenantID, id)
	}))
	m.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func removeLogFields(line string, fields ...string) string {
	for _, field := range fields {
		// find field
		needle := field + "="
		pos := strings.Index(line, needle)
		if pos < 0 {
			continue
		}

		// find space after field
		offset := pos + len(needle)
		posSpace := strings.Index(line[offset:], " ")
		if posSpace < 0 {
			// remove all after needle
			line = line[0:offset]
			continue
		}

		// remove value
		line = line[:offset] + line[offset+posSpace:]
	}

	return line

}

type errorRecorder struct {
	writeErr error
}

func (r *errorRecorder) Write([]byte) (int, error) { return 0, r.writeErr }

func (*errorRecorder) Header() http.Header { return make(http.Header) }

func (*errorRecorder) WriteHeader(statusCode int) {}

func TestHTTPLog(t *testing.T) {
	ctxTenant := user.InjectOrgID(context.Background(), "my-tenant")
	for _, tc := range []struct {
		name          string
		log           *Log
		ctx           context.Context
		reqBody       io.Reader
		writeErr      error
		setHeaderList []string
		statusCode    int
		message       string
	}{
		{
			name: "Header logging disabled",
			log: &Log{
				LogRequestHeaders: false,
			},
			setHeaderList: []string{"good-header", "authorization"},
			message:       `level=debug method=GET uri=http://example.com/foo status=200 duration= msg="http request processed"`,
		},
		{
			name: "Header logging enable",
			log: &Log{
				LogRequestHeaders: true,
			},
			setHeaderList: []string{"good-header", "authorization"},
			message:       `level=debug method=GET uri=http://example.com/foo status=200 duration= request_header_Good-Header=good-headerValue msg="http request processed"`,
		},
		{
			name: "Extra Header excluded",
			log: &Log{
				LogRequestHeaders:        true,
				LogRequestExcludeHeaders: []string{"bad-header"},
			},
			setHeaderList: []string{"good-header", "bad-header", "authorization"},
			message:       `level=debug method=GET uri=http://example.com/foo status=200 duration= request_header_Good-Header=good-headerValue msg="http request processed"`,
		},
		{
			name: "Extra Header with different casing",
			log: &Log{
				LogRequestHeaders:        true,
				LogRequestExcludeHeaders: []string{"Bad-Header"},
			},
			setHeaderList: []string{"good-header", "bad-header", "authorization"},
			message:       `level=debug method=GET uri=http://example.com/foo status=200 duration= request_header_Good-Header=good-headerValue msg="http request processed"`,
		},
		{
			name: "Two Extra Headers excluded",
			log: &Log{
				LogRequestHeaders:        true,
				LogRequestExcludeHeaders: []string{"bad-header", "bad-header2"},
			},
			setHeaderList: []string{"good-header", "bad-header", "bad-header2", "authorization"},
			message:       `level=debug method=GET uri=http://example.com/foo status=200 duration= request_header_Good-Header=good-headerValue msg="http request processed"`,
		},
		{
			name: "Status code 500 should still log headers",
			log: &Log{
				LogRequestHeaders:        false,
				LogRequestExcludeHeaders: []string{"bad-header"},
			},
			setHeaderList: []string{"good-header", "bad-header", "authorization"},
			message:       `level=warn method=GET uri=http://example.com/foo status=500 duration= request_header_Good-Header=good-headerValue msg="http request failed" response_body="<html><body>Hello world!</body></html>"`,

			statusCode: http.StatusInternalServerError,
		},
		{
			name:    "Log request body size latency",
			log:     &Log{},
			reqBody: strings.NewReader("Hello World! I am a request body."),
			message: `level=debug method=GET uri=http://example.com/foo status=200 duration= request_body_size=33B request_body_read_duration= msg="http request processed"`,
		},
		{
			name:     "Write errors should be shown at warning level",
			log:      &Log{},
			writeErr: errors.New("some error"),
			message:  `level=warn method=GET uri=http://example.com/foo status=200 duration= msg="http request failed" err="some error"`,
		},
		{
			name:     "Context cancelled requests should not be at warning level",
			log:      &Log{},
			writeErr: context.Canceled,
			message:  `level=debug method=GET uri=http://example.com/foo status=200 duration= msg="request cancelled"`,
		},
		{
			name:    "Tenant id should be logged",
			ctx:     ctxTenant,
			log:     &Log{},
			message: `level=debug tenant=my-tenant method=GET uri=http://example.com/foo status=200 duration= msg="http request processed"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)

			if tc.statusCode == 0 {
				tc.statusCode = http.StatusOK
			}

			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			tc.log.Log = log.NewLogfmtLogger(buf)

			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.Body != nil {
					_, _ = io.Copy(io.Discard, r.Body)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, "<html><body>Hello world!</body></html>")
			}
			loggingHandler := tc.log.Wrap(http.HandlerFunc(handler))

			req := httptest.NewRequestWithContext(ctx, "GET", "http://example.com/foo", tc.reqBody)
			for _, header := range tc.setHeaderList {
				req.Header.Set(header, header+"Value")
			}

			var recorder http.ResponseWriter = httptest.NewRecorder()
			if tc.writeErr != nil {
				recorder = &errorRecorder{writeErr: tc.writeErr}
			}
			loggingHandler.ServeHTTP(recorder, req)

			output := buf.String()
			assert.Equal(t, tc.message, removeLogFields(strings.TrimSpace(output), "duration", "request_body_read_duration"))
		})
	}
}
