package util_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/util"
)

func TestWriteTextResponse(t *testing.T) {
	w := httptest.NewRecorder()

	util.WriteTextResponse(w, "hello world")

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "hello world", w.Body.String())
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
}

func TestMultitenantMiddleware(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://localhost:8080", nil)

	// No org ID header.
	m := util.AuthenticateUser(true).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	m = util.AuthenticateUser(false).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := tenant.ExtractTenantIDFromContext(r.Context())
		require.NoError(t, err)
		assert.Equal(t, tenant.DefaultTenantID, id)
	}))
	m.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}
