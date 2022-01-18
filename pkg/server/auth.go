package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

// TODO(kolesnikovae):
//  - AuthHandler is to be moved to pkg/api.
//  - Other handler are to be decoupled with Controller and to be moved as well.

//go:generate mockgen -destination mocks/auth.go -package mocks . AuthService

type AuthService interface {
	APIKeyFromJWTToken(context.Context, string) (model.TokenAPIKey, error)
	UserFromJWTToken(context.Context, string) (model.User, error)
}

type AuthHandler struct {
	log         logrus.FieldLogger
	authService AuthService

	// TODO(kolesnikovae): Consider moving redirect logic to the client.
	loginRedirect http.HandlerFunc
}

func NewAuthHandler(
	logger logrus.FieldLogger,
	authService AuthService,
	loginRedirect http.HandlerFunc) AuthHandler {
	return AuthHandler{
		log:           logger,
		authService:   authService,
		loginRedirect: loginRedirect,
	}
}

// Middleware authenticates the request.
func (s AuthHandler) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := s.log.WithFields(logrus.Fields{
			"url":  r.URL.String(),
			"host": r.Header.Get("Host"),
		})

		if token, ok := extractTokenFromAuthHeader(r.Header.Get("Authorization")); ok {
			if err := s.withAPIKeyFromToken(r, token); err != nil {
				logger.WithError(err).Debug("invalid api key")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if c, err := r.Cookie(jwtCookieName); err == nil {
			if err = s.withUserFromToken(r, c.Value); err != nil {
				logger.WithError(err).Debug("invalid jwt cookie")
				s.loginRedirect(w, r)
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
		s.loginRedirect(w, r)
	}
}

// withUserFromToken retrieves User of the given token t and enriches the
// request context. Obtained user can be accessed from the handler
// via the `model.UserFromContext` call.
//
// The method fails if the token is invalid or the user can't be authenticated
// (e.g. can not be found or is disabled).
func (s AuthHandler) withUserFromToken(r *http.Request, t string) error {
	ctx := r.Context()
	u, err := s.authService.UserFromJWTToken(ctx, t)
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
func (s AuthHandler) withAPIKeyFromToken(r *http.Request, t string) error {
	ctx := r.Context()
	k, err := s.authService.APIKeyFromJWTToken(ctx, t)
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
