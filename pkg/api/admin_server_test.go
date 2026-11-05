package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/server"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAPI builds a minimal API wired to a gorilla mux, mirroring the pattern
// used in distributor_api_test.go.
func newTestAPIWithMode(t *testing.T, mode AdminServerMode) (*API, *mux.Router, *mux.Router) {
	t.Helper()

	mainMux := mux.NewRouter()
	serv := &server.Server{HTTP: mainMux}

	a, err := New(Config{}, serv, grpcgw.NewServeMux(), log.NewNopLogger())
	require.NoError(t, err)

	adminRouter := mux.NewRouter()
	a.SetAdminRouter(adminRouter, mode)

	return a, mainMux, adminRouter
}

// probe sends a GET to the given router and returns the status code.
func probe(t *testing.T, router http.Handler, path string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestRegisterInternalRoute_Disabled(t *testing.T) {
	a, mainMux, adminRouter := newTestAPIWithMode(t, AdminServerDisabled)

	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	a.registerAdminRoute("/test-route", sentinel, WithMethod("GET"))

	// Route must be reachable on the main mux.
	assert.Equal(t, http.StatusTeapot, probe(t, mainMux, "/test-route"))
	// Route must NOT be reachable on the internal router.
	assert.Equal(t, http.StatusNotFound, probe(t, adminRouter, "/test-route"))
}

func TestRegisterInternalRoute_Exclusive(t *testing.T) {
	a, mainMux, adminRouter := newTestAPIWithMode(t, AdminServerExclusive)

	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	a.registerAdminRoute("/test-route", sentinel, WithMethod("GET"))

	// Route must NOT be on the main mux.
	assert.Equal(t, http.StatusNotFound, probe(t, mainMux, "/test-route"))
	// Route must be reachable on the internal router.
	assert.Equal(t, http.StatusTeapot, probe(t, adminRouter, "/test-route"))
}

func TestRegisterInternalRoute_Additional(t *testing.T) {
	a, mainMux, adminRouter := newTestAPIWithMode(t, AdminServerAdditional)

	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	a.registerAdminRoute("/test-route", sentinel, WithMethod("GET"))

	// Route must be reachable on both muxes.
	assert.Equal(t, http.StatusTeapot, probe(t, mainMux, "/test-route"))
	assert.Equal(t, http.StatusTeapot, probe(t, adminRouter, "/test-route"))
}

func TestRegisterInternalRoute_PathParameters(t *testing.T) {
	// Verify that gorilla mux path parameters work correctly on the internal router
	// (they would silently break with http.ServeMux).
	a, _, adminRouter := newTestAPIWithMode(t, AdminServerExclusive)

	var capturedTenant string
	a.registerAdminRoute("/ops/tenants/{tenant}/blocks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTenant = mux.Vars(r)["tenant"]
		w.WriteHeader(http.StatusOK)
	}), WithMethod("GET"))

	req := httptest.NewRequest(http.MethodGet, "/ops/tenants/acme/blocks", nil)
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "acme", capturedTenant)
}

func TestSetAdminRouter_NilSafe(t *testing.T) {
	// When no internal router is set, registerAdminRoute should fall back to
	// the main mux without panicking.
	mainMux := mux.NewRouter()
	serv := &server.Server{HTTP: mainMux}
	a, err := New(Config{}, serv, grpcgw.NewServeMux(), log.NewNopLogger())
	require.NoError(t, err)
	// adminRouter is nil, mode is "" (zero value = disabled path)

	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	require.NotPanics(t, func() {
		a.registerAdminRoute("/nil-safe-route", sentinel, WithMethod("GET"))
	})
	assert.Equal(t, http.StatusTeapot, probe(t, mainMux, "/nil-safe-route"))
}
