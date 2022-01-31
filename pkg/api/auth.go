package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

const jwtCookieName = "pyroscopeJWT"

//go:generate mockgen -destination mocks/auth.go -package mocks . AuthService

type AuthService interface {
	APIKeyFromJWTToken(ctx context.Context, token string) (model.TokenAPIKey, error)
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
				ctx, err := withAPIKeyFromToken(authService, r.Context(), token)
				if err != nil {
					logger.WithError(err).Debug("failed to authenticate api key")
					Error(w, model.ErrInvalidCredentials)
					return
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if c, err := r.Cookie(jwtCookieName); err == nil {
				ctx, err := withUserFromToken(authService, r.Context(), c.Value)
				if err != nil {
					logger.WithError(err).Debug("failed to authenticate jwt cookie")
					// Error(w, model.ErrInvalidCredentials)
					loginRedirect(w, r)
					return
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// For backward compatibility, on failure we assume the
			// requester is user and redirect them to the login page,
			// which is not appropriate for API key authentication
			// and may confuse end users.
			logger.Debug("unauthenticated request")
			// Error(w, ErrAuthenticationRequired)
			loginRedirect(w, r)
		})
	}
}

// withUserFromToken retrieves User of the given token t and enriches the
// request context. Obtained user can be accessed from the handler
// via the `model.UserFromContext` call.
//
// The method fails if the token is invalid or the user can't be authenticated
// (e.g. can not be found or is disabled).
func withUserFromToken(s AuthService, ctx context.Context, t string) (context.Context, error) {
	u, err := s.UserFromJWTToken(ctx, t)
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
// authenticated (e.g. can not be found, expired, or it's signature
// has changed).
func withAPIKeyFromToken(s AuthService, ctx context.Context, t string) (context.Context, error) {
	k, err := s.APIKeyFromJWTToken(ctx, t)
	if err != nil {
		return nil, err
	}
	return model.WithTokenAPIKey(ctx, k), nil
}

const bearer string = "bearer"

func extractTokenFromAuthHeader(val string) (token string, ok bool) {
	authHeaderParts := strings.Split(val, " ")
	if len(authHeaderParts) != 2 || !strings.EqualFold(authHeaderParts[0], bearer) {
		return "", false
	}
	return authHeaderParts[1], true
}
