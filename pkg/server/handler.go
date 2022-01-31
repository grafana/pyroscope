package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"text/template"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
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
		"BasicAuthEnabled":       ctrl.config.Auth.BasicAuth.Enabled,
		"BasicAuthSignupEnabled": ctrl.config.Auth.BasicAuth.SignupEnabled,
		"GoogleEnabled":          ctrl.config.Auth.Google.Enabled,
		"GithubEnabled":          ctrl.config.Auth.Github.Enabled,
		"GitlabEnabled":          ctrl.config.Auth.Gitlab.Enabled,
		"BaseURL":                ctrl.config.BaseURL,
	})
}

func (ctrl *Controller) loginPost(w http.ResponseWriter, r *http.Request) {
	if !ctrl.config.Auth.BasicAuth.Enabled {
		http.Error(w, "not authorized", http.StatusUnauthorized)
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
	createCookie(w, jwtCookieName, token)
	w.WriteHeader(http.StatusNoContent)
	// Redirect should be handled on the client side.
}

func (ctrl *Controller) signupHandler(w http.ResponseWriter, r *http.Request) {
	if !ctrl.config.Auth.BasicAuth.SignupEnabled {
		http.Error(w, "not authorized", http.StatusUnauthorized)
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
		Email:    req.Email,
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

func createCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: http.SameSiteStrictMode,
	})
}

func invalidateCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
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
		authURL, err := oh.getOauthBase().buildAuthQuery(r, w)
		if err != nil {
			ctrl.log.WithError(err).Error("problem generating state token")
			return
		}

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
				Email:      u.Email,
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

		token, _, err := ctrl.jwtTokenService.Sign(ctrl.jwtTokenService.GenerateUserToken(user.Name, user.Role))
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		createCookie(w, jwtCookieName, token)
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

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	fs := http.FileServer(ctrl.dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/comparison" {
			ctrl.statsInc("comparison")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/comparison-diff" {
			ctrl.statsInc("diff")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-single" {
			ctrl.statsInc("adhoc-index")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-comparison" {
			ctrl.statsInc("adhoc-comparison")
			ctrl.renderIndexPage(rw, r)
		} else if path == "/adhoc-comparison-diff" {
			ctrl.statsInc("adhoc-comparison-diff")
			ctrl.renderIndexPage(rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}

func (ctrl *Controller) getTemplate(path string) (*template.Template, error) {
	f, err := ctrl.dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %q", path, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %q", path, err)
	}

	tmpl, err := template.New(path).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s template: %q", path, err)
	}
	return tmpl, nil
}

func (ctrl *Controller) renderIndexPage(w http.ResponseWriter, _ *http.Request) {
	tmpl, err := ctrl.getTemplate("/index.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render index page")
		return
	}

	initialStateObj := indexPageJSON{}
	initialStateObj.AppNames = ctrl.storage.GetAppNames()

	var b []byte
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}

	initialStateStr := string(b)
	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = os.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Errorf("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	// Feature Flags
	// Add this intermediate layer instead of just exposing as it comes from ctrl.config
	// Since we may probably want to rename these flags when exposing to the frontend
	features := struct {
		EnableExperimentalAdhocUI bool `json:"enableExperimentalAdhocUI"`
	}{
		EnableExperimentalAdhocUI: ctrl.config.EnableExperimentalAdhocUI,
	}
	b, err = json.Marshal(features)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}
	featuresStr := string(b)

	w.Header().Add("Content-Type", "text/html")
	mustExecute(tmpl, w, map[string]string{
		"InitialState":      initialStateStr,
		"BuildInfo":         build.JSON(),
		"LatestVersionInfo": updates.LatestVersionJSON(),
		"ExtraMetadata":     extraMetadataStr,
		"BaseURL":           ctrl.config.BaseURL,
		"NotificationText":  ctrl.notifier.NotificationText(),
		"IsAuthRequired":    strconv.FormatBool(ctrl.isAuthRequired()),
		"Features":          featuresStr,
	})
}

func mustExecute(t *template.Template, w io.Writer, v interface{}) {
	if err := t.Execute(w, v); err != nil {
		panic(err)
	}
}
