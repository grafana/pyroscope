package delayhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/pyroscope/pkg/tenant"
)

// Mock implementation of the Limits interface
type mockLimits struct {
	delays map[string]time.Duration
}

func (m *mockLimits) IngestionArtificialDelay(tenantID string) time.Duration {
	if delay, ok := m.delays[tenantID]; ok {
		return delay
	}
	return 0
}

func newMockLimits() *mockLimits {
	return &mockLimits{
		delays: make(map[string]time.Duration),
	}
}

func (m *mockLimits) setDelay(tenantID string, delay time.Duration) {
	m.delays[tenantID] = delay
}

// Test handler that records what happened
type testHandler struct {
	statusCode  int
	body        string
	called      bool
	cancelDelay bool
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	if h.cancelDelay {
		CancelDelay(r.Context())
	}
	if h.statusCode != 0 {
		w.WriteHeader(h.statusCode)
	}
	if h.body != "" {
		_, _ = w.Write([]byte(h.body))
	}
}

func timeNowMock(t *testing.T, values []time.Time) func() {
	old := timeNow
	t.Cleanup(func() {
		timeNow = old
	})
	timeNow = func() time.Time {
		if len(values) == 0 {
			t.Fatalf("timeNowMock: no more values")
		}
		now := values[0]
		values = values[1:]
		return now
	}
	return func() {

		timeNow = old
	}
}

type timeAfterRecorder struct {
	start  time.Time
	values []time.Duration
}

func timeAfterMock() (*timeAfterRecorder, func()) {
	m := &timeAfterRecorder{
		start: time.Now(),
	}
	old := timeAfter
	timeAfter = func(d time.Duration) <-chan time.Time {
		m.values = append(m.values, d)

		ch := make(chan time.Time)
		go func() {
			ch <- m.start.Add(d)
		}()

		return ch
	}
	return m, func() {
		timeAfter = old
	}
}

func fracDuration(d time.Duration, frac float64) time.Duration {
	return time.Duration(int64(float64(d.Milliseconds())*frac)) * time.Millisecond
}

func TestNewHTTP(t *testing.T) {
	now := time.Unix(1718211600, 0)
	tenantID := "my-tenant"

	tests := []struct {
		name              string
		configDelay       time.Duration
		handlerStatusCode int
		handlerBody       string
		handlerDelay      time.Duration // delay in handler
		middlewareDelay   time.Duration // delay in other middlewares
		cancelDelay       bool
		expectDelay       bool
		expectDelayHeader bool
	}{
		{
			name:              "enabled/successful request",
			configDelay:       100 * time.Millisecond,
			handlerBody:       "success",
			expectDelay:       true,
			expectDelayHeader: true,
		},
		{
			name:        "disabled/successful request",
			configDelay: 0,
			handlerBody: "success",
		},
		{
			name:              "enabled/successful request/written headers",
			configDelay:       100 * time.Millisecond,
			handlerStatusCode: http.StatusOK,
			handlerBody:       "success",
			expectDelay:       true,
			expectDelayHeader: true,
		},
		{
			name:              "enabled/failed request/written headers",
			configDelay:       100 * time.Millisecond,
			handlerStatusCode: http.StatusInternalServerError,
			handlerBody:       "error",
		},
		{
			name:              "disabled/failed request/written headers",
			handlerStatusCode: http.StatusInternalServerError,
			handlerBody:       "error",
		},
		{
			name:         "enabled/successful slow request",
			configDelay:  100 * time.Millisecond,
			handlerBody:  "slow handler success",
			handlerDelay: 200 * time.Millisecond,
		},
		{
			name:              "enabled/successful slow request/written headers",
			configDelay:       100 * time.Millisecond,
			handlerStatusCode: http.StatusOK,
			handlerBody:       "slow handler success",
			handlerDelay:      200 * time.Millisecond,
		},
		{
			name:              "enabled/successful request/written headers/slow middleware",
			configDelay:       100 * time.Millisecond,
			handlerStatusCode: http.StatusOK,
			handlerBody:       "slow middlewares success",
			middlewareDelay:   200 * time.Millisecond,
			expectDelayHeader: true,
		},
		{
			name:            "enabled/successful request/slow middleware",
			configDelay:     100 * time.Millisecond,
			handlerBody:     "slow middlewares success",
			middlewareDelay: 200 * time.Millisecond,
		},
		{
			name:        "enabled/cancel delay",
			configDelay: 100 * time.Millisecond,
			handlerBody: "success",
			cancelDelay: true,
		},
		{
			name:        "disabled/cancel delay no effect",
			configDelay: 0,
			handlerBody: "success",
			cancelDelay: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerDelay := tt.handlerDelay
			if handlerDelay == 0 {
				handlerDelay = 5 * time.Millisecond
			}
			middlewareDelay := tt.middlewareDelay
			if middlewareDelay == 0 {
				middlewareDelay = 1 * time.Millisecond
			}

			// start of handler
			nows := []time.Time{
				now,
			}

			// when a upstream handler writes headers, we will have an extra timeNow call
			if tt.handlerStatusCode != 0 {
				nows = append(nows, now.Add(handlerDelay))
			}
			// final time now check including both delays
			nows = append(nows, now.Add(handlerDelay+middlewareDelay))

			// mock timeNow and timeAfter
			cleanUpNow := timeNowMock(t, nows)
			defer cleanUpNow()
			sleeps, cleanUpSleep := timeAfterMock()
			defer cleanUpSleep()
			sleeps.start = now

			limits := newMockLimits()
			limits.setDelay(tenantID, tt.configDelay)
			middleware := NewHTTP(limits)

			handler := &testHandler{
				statusCode:  tt.handlerStatusCode,
				body:        tt.handlerBody,
				cancelDelay: tt.cancelDelay,
			}

			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("test"))
			req = req.WithContext(tenant.InjectTenantID(req.Context(), "my-tenant"))
			w := httptest.NewRecorder()

			middleware(handler).ServeHTTP(w, req)

			// Verify handler was called
			assert.True(t, handler.called)

			// Verify response
			expectedStatusCode := tt.handlerStatusCode
			if expectedStatusCode == 0 {
				expectedStatusCode = http.StatusOK
			}
			assert.Equal(t, expectedStatusCode, w.Code)
			assert.Equal(t, tt.handlerBody, w.Body.String())

			// Expect header or no header depending on delay
			if tt.expectDelayHeader {
				serverTiming := w.Header().Get("Server-Timing")
				require.Contains(t, serverTiming, "artificial_delay")
				require.Contains(t, serverTiming, "dur=")
				idx := strings.Index(serverTiming, "dur=")

				durationFloat, err := strconv.ParseFloat(serverTiming[idx+4:], 64)
				duration := time.Duration(durationFloat) * time.Millisecond
				assert.NoError(t, err)

				assert.Greater(t, duration, fracDuration(tt.configDelay, 0.8)-handlerDelay-middlewareDelay)
				assert.Greater(t, fracDuration(tt.configDelay, 1.1), duration)
			} else {
				serverTiming := w.Header().Get("Server-Timing")
				assert.Empty(t, serverTiming)
			}

			// Expect sleeps when delayed
			if tt.expectDelay {
				require.Len(t, sleeps.values, 1)

				// check if delay is within jitter of expected
				assert.Greater(t, sleeps.values[0], fracDuration(tt.configDelay, 0.8)-handlerDelay-middlewareDelay)
				assert.Greater(t, fracDuration(tt.configDelay, 1.1), sleeps.values[0])
			} else {
				require.Len(t, sleeps.values, 0)
			}

		})
	}
}

type healthMock struct {
	grpc_health_v1.UnimplementedHealthServer
	called bool
}

func (h *healthMock) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	h.called = true
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func TestGRPCHandler(t *testing.T) {
	limits := newMockLimits()
	limits.setDelay("my-tenant", 100*time.Millisecond)
	delayMiddleware := middleware.Func(func(h http.Handler) http.Handler {
		return NewHTTP(limits)(h)
	})

	sleeps, cleanUpSleep := timeAfterMock()
	defer cleanUpSleep()

	addTenantMiddleware := middleware.Func(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(tenant.InjectTenantID(r.Context(), "my-tenant"))
			h.ServeHTTP(w, r)
		})
	})

	h2cMiddleware := middleware.Func(func(h http.Handler) http.Handler {
		return h2c.NewHandler(h, &http2.Server{})
	})

	grpcServer := grpc.NewServer()
	healthM := &healthMock{}
	grpc_health_v1.RegisterHealthServer(grpcServer, healthM)

	handler := middleware.Merge(
		h2cMiddleware,
		addTenantMiddleware,
		delayMiddleware,
	).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grpcServer.ServeHTTP(w, r)
	}))

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	// Set up a connection to the server.
	u, err := url.Parse(httpServer.URL)
	require.NoError(t, err)
	conn, err := grpc.NewClient(u.Host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: "pyroscope"})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
	assert.True(t, healthM.called)

	// check if the delay is applied
	require.Len(t, sleeps.values, 1)
}
