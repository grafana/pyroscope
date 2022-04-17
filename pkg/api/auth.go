package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

const JWTCookieName = "pyroscopeJWT"

//go:generate mockgen -destination mocks/auth.go -package mocks . AuthService

type AuthService interface {
	APIKeyFromToken(ctx context.Context, token string) (model.APIKey, error)
	UserFromJWTToken(ctx context.Context, token string) (model.User, error)
	AuthenticateUser(ctx context.Context, name, password string) (model.User, error)
}

// AuthMiddleware authenticates requests.
func AuthMiddleware(loginRedirect http.HandlerFunc, authService AuthService, h httputils.Utils) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := extractTokenFromAuthHeader(r.Header.Get("Authorization")); ok {
				k, err := authService.APIKeyFromToken(r.Context(), token)
				if err != nil {
					h.HandleError(w, r, model.AuthenticationError{Err: err})
					return
				}
				next.ServeHTTP(w, r.WithContext(model.WithAPIKey(r.Context(), k)))
				return
			}

			if c, err := r.Cookie(JWTCookieName); err == nil {
				var u model.User
				if u, err = authService.UserFromJWTToken(r.Context(), c.Value); err != nil {
					if loginRedirect != nil {
						h.Logger(r).WithError(err).Debug("failed to authenticate jwt cookie")
						loginRedirect(w, r)
						return
					}
					h.HandleError(w, r, model.AuthenticationError{Err: err})
					return
				}
				next.ServeHTTP(w, r.WithContext(model.WithUser(r.Context(), u)))
				return
			}

			h.Logger(r).Debug("unauthenticated request")
			if loginRedirect != nil {
				loginRedirect(w, r)
				return
			}

			h.HandleError(w, r, model.ErrCredentialsInvalid)
		})
	}
}

const bearer string = "bearer"

func extractTokenFromAuthHeader(val string) (token string, ok bool) {
	authHeaderParts := strings.Split(val, " ")
	if len(authHeaderParts) != 2 || !strings.EqualFold(authHeaderParts[0], bearer) {
		return "", false
	}
	return authHeaderParts[1], true
}
