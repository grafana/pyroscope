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
	APIKeyFromJWTToken(context.Context, string) (model.TokenAPIKey, error)
	UserFromJWTToken(context.Context, string) (model.User, error)
}

// AuthMiddleware authenticates requests.
func AuthMiddleware(log logrus.FieldLogger, loginRedirect http.HandlerFunc, authService AuthService) func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			logger := log.WithFields(logrus.Fields{
				"remote": r.RemoteAddr,
				"url":    r.URL.String(),
			})

			if token, ok := extractTokenFromAuthHeader(r.Header.Get("Authorization")); ok {
				if err := withAPIKeyFromToken(authService, r, token); err != nil {
					logger.WithError(err).Debug("invalid api key")
					Error(w, ErrCredentialsInvalid)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if c, err := r.Cookie(jwtCookieName); err == nil {
				if err = withUserFromToken(authService, r, c.Value); err != nil {
					logger.WithError(err).Debug("invalid jwt cookie")
					// Error(w, ErrCredentialsInvalid)
					loginRedirect(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// For backward compatibility, on failure we assume the
			// requester is user and redirect them to the login page,
			// which is not appropriate for API key authentication
			// and may confuse end users.
			logger.Debug("unauthenticated request")
			// Error(w, ErrAuthenticationRequired)
			loginRedirect(w, r)
		}
	}
}

// withUserFromToken retrieves User of the given token t and enriches the
// request context. Obtained user can be accessed from the handler
// via the `model.UserFromContext` call.
//
// The method fails if the token is invalid or the user can't be authenticated
// (e.g. can not be found or is disabled).
func withUserFromToken(s AuthService, r *http.Request, t string) error {
	ctx := r.Context()
	u, err := s.UserFromJWTToken(ctx, t)
	if err != nil {
		return err
	}
	r = r.WithContext(model.WithUser(ctx, u))
	return nil
}

// withAPIKeyFromToken retrieves API key for the given token t and
// enriches the request context. Obtained API key then can be accessed
// from the handler via the `model.APIKeyFromContext` call.
//
// The method fails if the token is invalid or the API key can't be
// authenticated (e.g. can not be found, expired, or it's signature
// has changed).
func withAPIKeyFromToken(s AuthService, r *http.Request, t string) error {
	ctx := r.Context()
	k, err := s.APIKeyFromJWTToken(ctx, t)
	if err != nil {
		return err
	}
	r = r.WithContext(model.WithTokenAPIKey(ctx, k))
	return nil
}

const bearer string = "bearer"

func extractTokenFromAuthHeader(val string) (token string, ok bool) {
	authHeaderParts := strings.Split(val, " ")
	if len(authHeaderParts) != 2 || !strings.EqualFold(authHeaderParts[0], bearer) {
		return "", false
	}
	return authHeaderParts[1], true
}
