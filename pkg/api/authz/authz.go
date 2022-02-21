package authz

import (
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var (
	RequireAdminRole         = Require(Role(model.AdminRole))
	RequireAuthenticatedUser = Require(AuthenticatedUser)
)

// AllowAny does not verify if a request is authorized.
func AllowAny(next http.Handler) http.Handler { return next }

func Require(funcs ...func(r *http.Request) bool) func(next http.Handler) http.Handler {
	if len(funcs) == 0 {
		panic("authorization method should be specified explicitly")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, fn := range funcs {
				if !fn(r) {
					api.Error(w, model.ErrPermissionDenied)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Role verifies if the identity (user or API key) associated
// with the request has the given role.
func Role(role model.Role) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		ctx := r.Context()
		if k, ok := model.APIKeyFromContext(ctx); ok {
			return k.Role == role
		}
		if u, ok := model.UserFromContext(ctx); ok {
			return u.Role == role
		}
		return false
	}
}

// AuthenticatedUser authorizes any authenticated user.
//
// Note that authenticated API key is not linked to any user,
// therefore this check will fail.
func AuthenticatedUser(r *http.Request) bool {
	_, ok := model.UserFromContext(r.Context())
	return ok
}
