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

func Role(role model.Role) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		return model.MustUserFromContext(r.Context()).Role == role
	}
}

// AuthenticatedUser authorizes any authenticated user.
//
// Note that authenticated API key is not linked to a user,
// therefore this check will fail.
func AuthenticatedUser(r *http.Request) bool {
	_, ok := model.UserFromContext(r.Context())
	return ok
}
