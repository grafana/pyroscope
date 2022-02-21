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
	APIKeyFromToken(ctx context.Context, token string) (model.APIKeyToken, error)
	UserFromJWTToken(ctx context.Context, token string) (model.User, error)
	AuthenticateUser(ctx context.Context, name, password string) (model.User, error)
}

// AuthMiddleware authenticates requests.
func AuthMiddleware(log logrus.FieldLogger, loginRedirect http.HandlerFunc, authService AuthService) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := log.WithFields(logrus.Fields{
				"remote": r.RemoteAddr,
				"url":    r.URL.String(),
			})

			if token, ok := extractTokenFromAuthHeader(r.Header.Get("Authorization")); ok {
				ctx, err := withAPIKeyFromToken(r.Context(), authService, token)
				if err != nil {
					logger.WithError(err).Debug("failed to authenticate api key")
					Error(w, model.ErrInvalidCredentials)
					return
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if c, err := r.Cookie(JWTCookieName); err == nil {
				ctx, err := withUserFromToken(r.Context(), authService, c.Value)
				if err != nil {
					logger.WithError(err).Debug("failed to authenticate jwt cookie")
					if loginRedirect != nil {
						loginRedirect(w, r)
						return
					}
					Error(w, model.ErrInvalidCredentials)
					return
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			logger.Debug("unauthenticated request")
			if loginRedirect != nil {
				loginRedirect(w, r)
				return
			}
			Error(w, model.ErrInvalidCredentials)
		})
	}
}

// withUserFromToken retrieves User of the given token t and enriches the
// request context. Obtained user can be accessed from the handler
// via the `model.UserFromContext` call.
//
// The method fails if the token is invalid or the user can't be authenticated
// (e.g. can not be found or is disabled).
func withUserFromToken(ctx context.Context, as AuthService, t string) (context.Context, error) {
	u, err := as.UserFromJWTToken(ctx, t)
	if err != nil {
		return nil, err
	}
	return model.WithUser(ctx, u), nil
}

// withAPIKeyFromToken retrieves API key for the given token t and
// enriches the request context. Obtained API key then can be accessed
// from the handler via the `model.APIKeyFromContext` call.
//
// The method fails if the token is invalid or the API key can't be
// authenticated (e.g. can not be found or it's expired).
func withAPIKeyFromToken(ctx context.Context, as AuthService, t string) (context.Context, error) {
	k, err := as.APIKeyFromToken(ctx, t)
	if err != nil {
		return nil, err
	}
	return model.WithAPIKey(ctx, k), nil
}

const bearer string = "bearer"

func extractTokenFromAuthHeader(val string) (token string, ok bool) {
	authHeaderParts := strings.Split(val, " ")
	if len(authHeaderParts) != 2 || !strings.EqualFold(authHeaderParts[0], bearer) {
		return "", false
	}
	return authHeaderParts[1], true
}
