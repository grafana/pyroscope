package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/util/updates"
)

func (ctrl *Controller) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctrl.loginViewHandler(w, r)
	case http.MethodPost:
		ctrl.loginPostHandler(w, r)
	default:
		ctrl.writeInvalidMethodError(w)
		return
	}
}

func (ctrl *Controller) loginViewHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := ctrl.getTemplate("/login.html")
	if err != nil {
		ctrl.writeInternalServerError(w, err, "could not render login page")
		return
	}
	mustExecute(tmpl, w, map[string]interface{}{
		"SignupEnabled": ctrl.config.Auth.SignupEnabled,
		"BasicEnabled":  ctrl.config.Auth.Basic.Enabled,
		"GoogleEnabled": ctrl.config.Auth.Google.Enabled,
		"GithubEnabled": ctrl.config.Auth.Github.Enabled,
		"GitlabEnabled": ctrl.config.Auth.Gitlab.Enabled,
		"BaseURL":       ctrl.config.BaseURL,
	})
}

func (ctrl *Controller) loginPostHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(kolesnikovae):
	//  Retrieve user credentials from request
	//  Retrieve user by email or name
	//  Verify credentials
	//  Generate and inject token
	//  Redirect to index page
}

func (ctrl *Controller) signupHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(kolesnikovae):
	//  Insert user from request
	//  Generate and inject token
	//  Redirect to index page
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

func (ctrl *Controller) newJWTToken(claims jwt.MapClaims) (string, error) {
	if claims == nil {
		claims = jwt.MapClaims{}
	}
	claims["iat"] = time.Now().Unix()
	if ctrl.config.Auth.LoginMaximumLifetimeDays > 0 {
		claims["exp"] = time.Now().Add(time.Hour * 24 * time.Duration(ctrl.config.Auth.LoginMaximumLifetimeDays)).Unix()
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return jwtToken.SignedString([]byte(ctrl.config.Auth.JWTSecret))
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

		extUser, err := oh.userAuth(client)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "failed to get user auth info", err)
		}
		if extUser.Name == "" {
			extUser.Name = extUser.Email
		}

		tk, err := ctrl.newJWTToken(jwt.MapClaims{
			"name":  extUser.Name,
			"email": extUser.Email,
		})
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// TODO(kolesnikovae): Upsert user.

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		createCookie(w, jwtCookieName, tk)

		tmpl, err := ctrl.getTemplate("/welcome.html")
		if err != nil {
			ctrl.writeInternalServerError(w, err, "could not render welcome page")
			return
		}

		mustExecute(tmpl, w, map[string]interface{}{
			"Name":    extUser.Name,
			"Email":   extUser.Email,
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
