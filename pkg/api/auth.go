package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

const JWTCookieName = "pyroscopeJWT"

//go:generate mockgen -destination mocks/auth.go -package mocks . AuthService

type AuthService interface {
	APIKeyFromToken(ctx context.Context, token string) (model.APIKey, error)
	UserFromJWTToken(ctx context.Context, token string) (model.User, error)
	AuthenticateUser(ctx context.Context, name, password string) (model.User, error)
}

// AuthMiddleware authenticates requests.
func AuthMiddleware(log logrus.FieldLogger, loginRedirect http.HandlerFunc, authService AuthService) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := Logger(r, log)

			if token, ok := extractTokenFromAuthHeader(r.Header.Get("Authorization")); ok {
				k, err := authService.APIKeyFromToken(r.Context(), token)
				if err != nil {
					Error(w, logger, model.AuthenticationError{Err: err})
					return
				}
				next.ServeHTTP(w, r.WithContext(model.WithAPIKey(r.Context(), k)))
				return
			}

			if c, err := r.Cookie(JWTCookieName); err == nil {
				var u model.User
				if u, err = authService.UserFromJWTToken(r.Context(), c.Value); err != nil {
					if loginRedirect != nil {
						logger.WithError(err).Debug("failed to authenticate jwt cookie")
						loginRedirect(w, r)
						return
					}
					Error(w, logger, model.AuthenticationError{Err: err})
					return
				}
				next.ServeHTTP(w, r.WithContext(model.WithUser(r.Context(), u)))
				return
			}

			logger.Debug("unauthenticated request")
			if loginRedirect != nil {
				loginRedirect(w, r)
				return
			}

			Error(w, nil, model.ErrCredentialsInvalid)
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
