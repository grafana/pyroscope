package body

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/tenant"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/validation"
)

// Test handler that records what happened
type testHandler struct {
	called bool
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	n, err := io.Copy(io.Discard, r.Body)
	println(n)
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		httputil.ErrorWithStatus(w, err, http.StatusRequestEntityTooLarge)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func TestRequestBodyLimitMiddleware(t *testing.T) {
	tenantID := "my-tenant"
	anyByte := string('0')
	tests := []struct {
		name          string
		bodyLimit     int64
		bodySize      int
		expectedError bool
	}{
		{
			name:          "body size below limit",
			bodyLimit:     10,
			bodySize:      9,
			expectedError: false,
		},
		{
			name:          "body size matches limit",
			bodyLimit:     10,
			bodySize:      10,
			expectedError: false,
		},
		{
			name:          "body exceeds limit",
			bodyLimit:     10,
			bodySize:      11,
			expectedError: true,
		},
		{
			name:          "no limit set",
			bodyLimit:     0,
			bodySize:      11,
			expectedError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := validation.MockLimits{
				IngestionBodyLimitBytesValue: tt.bodyLimit,
			}
			middleware := NewSizeLimitHandler(limits)

			var handler testHandler
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(strings.Repeat(anyByte, tt.bodySize)))
			req = req.WithContext(tenant.InjectTenantID(req.Context(), tenantID))
			w := httptest.NewRecorder()

			middleware(&handler).ServeHTTP(w, req)

			// Verify handler was called
			assert.True(t, handler.called)

			if tt.expectedError {
				assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
			} else {
				assert.Equal(t, http.StatusOK, w.Code)
			}
		})
	}
}
