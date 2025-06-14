package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextKey uint8

const (
	contextKeyTest contextKey = iota
)

func newTestMiddleware(name string) middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			middlewares, ok := ctx.Value(contextKeyTest).([]string)
			if !ok {
				middlewares = []string{}
			}
			middlewares = append(middlewares, name)
			ctx = context.WithValue(ctx, contextKeyTest, middlewares)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

}

func Test_registerRoute(t *testing.T) {
	router := mux.NewRouter()
	registerRoute(
		log.NewNopLogger(),
		router,
		"/test",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewares := r.Context().Value(contextKeyTest).([]string)
			assert.Equal(t, []string{"outer", "middle", "inner"}, middlewares)

			w.WriteHeader(http.StatusOK)
		}),
		func(r *registerParams) {
			r.middlewares = append(r.middlewares, registerMiddleware{newTestMiddleware("outer"), "outer"})
		},
		func(r *registerParams) {
			r.middlewares = append(r.middlewares, registerMiddleware{newTestMiddleware("middle"), "middle"})
		},
		func(r *registerParams) {
			r.middlewares = append(r.middlewares, registerMiddleware{newTestMiddleware("inner"), "inner"})
		},
	)

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	req, err := http.NewRequest("GET", testServer.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := testServer.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
