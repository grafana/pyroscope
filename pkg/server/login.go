package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

// TODO(kolesnikovae): This part should be moved from
//  Controller to a separate handler/service (Login).

// TODO(kolesnikovae): Instead of rendering Login and Signup templates
//  on the server side in order to provide available auth options,
//  we should expose a dedicated endpoint, so that the client could
//  figure out all the necessary info on its own.

func (ctrl *Controller) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctrl.indexHandler()(w, r)
	case http.MethodPost:
		ctrl.loginPost(w, r)
	default:
		ctrl.httpUtils.WriteInvalidMethodError(r, w)
	}
}

func (ctrl *Controller) loginPost(w http.ResponseWriter, r *http.Request) {
	if !ctrl.isLoginWithPasswordAllowed(r) {
		ctrl.redirectPreservingBaseURL(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	type loginCredentials struct {
		Username string `json:"username"`
		Password []byte `json:"password"`
	}
	var req loginCredentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.log.WithError(err).Error("failed to parse user credentials")
		ctrl.httpUtils.HandleError(r, w, httputils.JSONError{Err: err})
		return
	}
	u, err := ctrl.authService.AuthenticateUser(r.Context(), req.Username, string(req.Password))
	if err != nil {
		ctrl.httpUtils.HandleError(r, w, err)
		return
	}
	token, err := ctrl.jwtTokenService.Sign(ctrl.jwtTokenService.GenerateUserJWTToken(u.Name, u.Role))
	if err != nil {
		ctrl.httpUtils.HandleError(r, w, err)
		return
	}
	ctrl.createCookie(w, api.JWTCookieName, token)
	w.WriteHeader(http.StatusNoContent)
}

func (ctrl *Controller) signupHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctrl.indexHandler()(w, r)
	case http.MethodPost:
		ctrl.signupPost(w, r)
	default:
		ctrl.httpUtils.WriteInvalidMethodError(r, w)
	}
}

func (ctrl *Controller) signupPost(w http.ResponseWriter, r *http.Request) {
	if !ctrl.isSignupAllowed(r) {
		ctrl.redirectPreservingBaseURL(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	type signupRequest struct {
		Name     string  `json:"name"`
		Email    *string `json:"email,omitempty"`
		FullName *string `json:"fullName,omitempty"`
		Password []byte  `json:"password"`
	}
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.httpUtils.HandleError(r, w, err)
		return
	}
	_, err := ctrl.userService.CreateUser(r.Context(), model.CreateUserParams{
		Name:     req.Name,
		Email:    req.Email,
		FullName: req.FullName,
		Password: string(req.Password),
		Role:     ctrl.config.Auth.SignupDefaultRole,
	})
	ctrl.httpUtils.HandleError(r, w, err)
}

func (ctrl *Controller) isAuthRequired() bool {
	return ctrl.config.Auth.Internal.Enabled ||
		ctrl.config.Auth.Google.Enabled ||
		ctrl.config.Auth.Github.Enabled ||
		ctrl.config.Auth.Gitlab.Enabled
}

func (ctrl *Controller) isLoginFormEnabled(r *http.Request) bool {
	return !ctrl.isUserAuthenticated(r) && ctrl.isAuthRequired()
}

func (ctrl *Controller) isLoginWithPasswordAllowed(r *http.Request) bool {
	return !ctrl.isUserAuthenticated(r) &&
		ctrl.config.Auth.Internal.Enabled
}

func (ctrl *Controller) isSignupAllowed(r *http.Request) bool {
	return !ctrl.isUserAuthenticated(r) &&
		ctrl.config.Auth.Internal.Enabled &&
		ctrl.config.Auth.Internal.SignupEnabled
}

func (ctrl *Controller) isUserAuthenticated(r *http.Request) bool {
	if v, err := r.Cookie(api.JWTCookieName); err == nil {
		if _, err = ctrl.authService.UserFromJWTToken(r.Context(), v.Value); err == nil {
			return true
		}
	}
	return false
}

func (ctrl *Controller) isCookieSecureRequired() bool {
	return ctrl.config.Auth.CookieSecure ||
		ctrl.config.Auth.CookieSameSite == http.SameSiteNoneMode
}

func (ctrl *Controller) createCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: ctrl.config.Auth.CookieSameSite,
		Secure:   ctrl.isCookieSecureRequired(),
	})
}

func (ctrl *Controller) invalidateCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge:   -1,
		SameSite: ctrl.config.Auth.CookieSameSite,
		Secure:   ctrl.isCookieSecureRequired(),
	})
}

func (ctrl *Controller) logoutHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodGet:
		ctrl.invalidateCookie(w, api.JWTCookieName)
		ctrl.loginRedirect(w, r)
	default:
		ctrl.httpUtils.WriteInvalidMethodError(r, w)
	}
}

// can be replaced with a faster solution if cryptographic randomness isn't a priority
func generateStateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (ctrl *Controller) oauthLoginHandler(oh oauthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authURL, state, err := oh.getOauthBase().buildAuthQuery(r, w)
		if err != nil {
			ctrl.log.WithError(err).Error("problem generating state token")
			return
		}
		ctrl.createCookie(w, stateCookieName, state)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}

// Instead of this handler that just redirects, Javascript code can be added to load the state and send it to backend
// this is done so that the state cookie would be send back from browser
func (ctrl *Controller) callbackHandler(redirectPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Well I know that's kinda kludge, but here is rationale:
		// We need to know if the url uses https or not
		// Previous way of doing this was to use the html template and detect the protocol
		// This way doesn't require any templates, but rely on referer header
		// The other way around would be to include the protocol in the baseURL
		hasTLS := "false"
		if strings.HasPrefix(r.Header.Get("Referer"), "https:") {
			hasTLS = "true"
		}

		ctrl.redirectPreservingBaseURL(w, r, redirectPath+"?"+r.URL.RawQuery+"&tls="+hasTLS, http.StatusPermanentRedirect)
	}
}

func (ctrl *Controller) logErrorAndRedirect(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if err != nil {
		ctrl.log.WithError(err).Error(msg)
	} else {
		ctrl.log.Error(msg)
	}
	ctrl.invalidateCookie(w, stateCookieName)
	ctrl.redirectPreservingBaseURL(w, r, "/forbidden", http.StatusTemporaryRedirect)
}

func (ctrl *Controller) callbackRedirectHandler(oh oauthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "missing state cookie", err)
			return
		}
		if cookie.Value != r.FormValue("state") {
			ctrl.logErrorAndRedirect(w, r, "invalid oauth state", nil)
			return
		}

		client, err := oh.getOauthBase().generateOauthClient(r)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "failed to generate oauth client", err)
			return
		}

		u, err := oh.userAuth(client)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "failed to get user auth info", err)
			return
		}

		user, err := ctrl.userService.FindUserByName(r.Context(), u.Name)
		switch {
		default:
			ctrl.logErrorAndRedirect(w, r, "failed to find user", err)
			return
		case err == nil:
			// TODO(kolesnikovae): Update found user with the new user info, if applicable.
		case errors.Is(err, model.ErrUserNotFound):
			user, err = ctrl.userService.CreateUser(r.Context(), model.CreateUserParams{
				Name:       u.Name,
				Email:      model.String(u.Email),
				Role:       ctrl.config.Auth.SignupDefaultRole,
				Password:   model.MustRandomPassword(),
				IsExternal: true,
				// TODO(kolesnikovae): Specify the user source (oauth-provider, ldap, etc).
			})
			if err != nil {
				ctrl.logErrorAndRedirect(w, r, "failed to create external user", err)
				return
			}
		}
		if model.IsUserDisabled(user) {
			ctrl.logErrorAndRedirect(w, r, "user disabled", err)
			return
		}
		token, err := ctrl.jwtTokenService.Sign(ctrl.jwtTokenService.GenerateUserJWTToken(user.Name, user.Role))
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// delete state cookie and add jwt cookie
		ctrl.invalidateCookie(w, stateCookieName)
		ctrl.createCookie(w, api.JWTCookieName, token)
		ctrl.redirectPreservingBaseURL(w, r, "/", http.StatusPermanentRedirect)
	}
}
