package authz

import (
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

func AllowAny(next http.HandlerFunc) http.HandlerFunc {
	return next.ServeHTTP
}

func Require(funcs ...func(r *http.Request) bool) func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for _, fn := range funcs {
				if !fn(r) {
					api.Error(w, api.ErrPermissionDenied)
					return
				}
			}
			next.ServeHTTP(w, r)
		}
	}
}

func AdminRole(r *http.Request) bool {
	return model.MustUserFromContext(r.Context()).Role == model.AdminRole
}
