package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

// TODO(kolesnikovae): This part should be moved from
//  Controller to a separate handler/service (Login).

func (ctrl *Controller) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctrl.loginGet(w)
	case http.MethodPost:
		ctrl.loginPost(w, r)
	default:
		ctrl.writeInvalidMethodError(w)
	}
}

func (ctrl *Controller) loginGet(w http.ResponseWriter) {
	tmpl, err := ctrl.getTemplate("/login.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render login page")
		return
	}
	mustExecute(tmpl, w, map[string]interface{}{
		"BasicAuthEnabled":       ctrl.config.Auth.Basic.Enabled,
		"BasicAuthSignupEnabled": ctrl.config.Auth.Basic.SignupEnabled,
		"GoogleEnabled":          ctrl.config.Auth.Google.Enabled,
		"GithubEnabled":          ctrl.config.Auth.Github.Enabled,
		"GitlabEnabled":          ctrl.config.Auth.Gitlab.Enabled,
		"BaseURL":                ctrl.config.BaseURL,
	})
}

func (ctrl *Controller) loginPost(w http.ResponseWriter, r *http.Request) {
	if !ctrl.config.Auth.Basic.Enabled {
		ctrl.logErrorAndRedirect(w, r, "basic authentication disabled", nil)
		return
	}
	type loginCredentials struct {
		Username string `json:"username"`
		Password []byte `json:"password"`
	}
	var req loginCredentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.log.WithError(err).Error("failed to parse user credentials")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := ctrl.authService.AuthenticateUser(r.Context(), req.Username, string(req.Password))
	switch {
	case err == nil:
		// Generate and sign new JWT token.
	case errors.Is(err, model.ErrInvalidCredentials):
		ctrl.log.WithError(err).Error("failed authentication attempt")
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	case model.IsValidationError(err):
		ctrl.log.WithError(err).Error("invalid authentication request")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		// Internal error.
		ctrl.log.WithError(err).Error("failed to authenticate user")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	token, _, err := ctrl.jwtTokenService.Sign(ctrl.jwtTokenService.GenerateUserToken(u.Name, u.Role))
	if err != nil {
		ctrl.log.WithError(err).Error("failed to generate user token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	ctrl.createCookie(w, jwtCookieName, token)
	w.WriteHeader(http.StatusNoContent)
	// Redirect should be handled on the client side.
}

func (ctrl *Controller) signupHandler(w http.ResponseWriter, r *http.Request) {
	if !ctrl.config.Auth.Basic.SignupEnabled {
		ctrl.logErrorAndRedirect(w, r, "signup disabled", nil)
		return
	}
	type signupRequest struct {
		Name     string     `json:"name"`
		Email    string     `json:"email"`
		FullName *string    `json:"fullName,omitempty"`
		Password []byte     `json:"password"`
		Role     model.Role `json:"role"`
	}
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.log.WithError(err).Error("failed to decode signup details")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := ctrl.userService.CreateUser(r.Context(), model.CreateUserParams{
		Name:     req.Name,
		Email:    model.String(req.Email),
		FullName: req.FullName,
		Password: string(req.Password),
		Role:     ctrl.config.Auth.SignupDefaultRole,
	})
	switch {
	case err == nil:
	case model.IsValidationError(err):
		ctrl.log.WithError(err).Debug("invalid signup details")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		ctrl.log.WithError(err).Error("failed to create user")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// TODO(kolesnikovae): We could generate a JWT token and set the cookie
	//  but it's better to just force user to login. A signup should be
	//  considered successful only when the user email is confirmed
	//  (not implemented yet).
	w.WriteHeader(http.StatusNoContent)
	// Redirect should be handled on the client side.
}

func (ctrl *Controller) createCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: ctrl.config.Auth.CookieSameSite,
	})
}

func invalidateCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge: -1,
	})
}

func (ctrl *Controller) logoutHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodGet:
		invalidateCookie(w, jwtCookieName)
		ctrl.redirectPreservingBaseURL(w, r, "/login", http.StatusTemporaryRedirect)
	default:
		ctrl.writeInvalidMethodError(w)
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
		tmpl, err := ctrl.getTemplate("/redirect.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render redirect page")
			return
		}
		mustExecute(tmpl, w, map[string]interface{}{
			"RedirectPath": redirectPath + "?" + r.URL.RawQuery,
			"BaseURL":      ctrl.config.BaseURL,
		})
	}
}

func (ctrl *Controller) forbiddenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := ctrl.getTemplate("/forbidden.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render forbidden page")
			return
		}
		mustExecute(tmpl, w, map[string]interface{}{
			"BaseURL": ctrl.config.BaseURL,
		})
	}
}

func (ctrl *Controller) logErrorAndRedirect(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if err != nil {
		ctrl.log.WithError(err).Error(msg)
	} else {
		ctrl.log.Error(msg)
	}
	invalidateCookie(w, stateCookieName)
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
		token, _, err := ctrl.jwtTokenService.Sign(ctrl.jwtTokenService.GenerateUserToken(user.Name, user.Role))
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		ctrl.createCookie(w, jwtCookieName, token)
		tmpl, err := ctrl.getTemplate("/welcome.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render welcome page")
			return
		}

		mustExecute(tmpl, w, map[string]interface{}{
			"Name":    u.Name,
			"BaseURL": ctrl.config.BaseURL,
		})
	}
}
