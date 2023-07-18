package authz

import (
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

type Authorizer struct {
	logger    logrus.FieldLogger
	httpUtils httputils.Utils
}

func NewAuthorizer(logger logrus.FieldLogger, httpUtils httputils.Utils) Authorizer {
	return Authorizer{
		logger:    logger,
		httpUtils: httpUtils,
	}
}

func (a Authorizer) RequireRole(role model.Role) func(next http.Handler) http.Handler {
	return a.Require(Role(role))
}

func (a Authorizer) RequireAdminRole(next http.Handler) http.Handler {
	return a.RequireRole(model.AdminRole)(next)
}

func (a Authorizer) RequireAuthenticatedUser(next http.Handler) http.Handler {
	return a.Require(AuthenticatedUser)(next)
}

func (a Authorizer) Require(funcs ...func(r *http.Request) bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, fn := range funcs {
				if !fn(r) {
					a.httpUtils.HandleError(r, w, model.ErrPermissionDenied)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (a Authorizer) RequireOneOf(funcs ...func(r *http.Request) bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, fn := range funcs {
				if fn(r) {
					next.ServeHTTP(w, r)
					return
				}
			}
			a.httpUtils.HandleError(r, w, model.ErrPermissionDenied)
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
// Also note that when auth is disabled this check also fails
func AuthenticatedUser(r *http.Request) bool {
	_, ok := model.UserFromContext(r.Context())
	return ok
}
