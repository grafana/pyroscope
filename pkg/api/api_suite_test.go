package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

func TestAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Suite")
}

// requestContextProvider wraps incoming request context.
// Mainly used to inject auth info; use defaultUserCtx if not sure.
type requestContextProvider func(context.Context) context.Context

var defaultUserCtx = ctxWithUser(&model.User{ID: 1, Role: model.AdminRole})

func ctxWithUser(u *model.User) requestContextProvider {
	return func(ctx context.Context) context.Context {
		return model.WithUser(ctx, *u)
	}
}

func ctxWithAPIKey(k *model.TokenAPIKey) requestContextProvider {
	return func(ctx context.Context) context.Context {
		return model.WithTokenAPIKey(ctx, *k)
	}
}

// newTestRouter initializes http router for testing purposes.
// It was decided to test the whole flow of the request handling,
// including routing, authentication, and authorization.
func newTestRouter(rcp requestContextProvider, services router.Services) *router.Router {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// For backward compatibility, the redirect handler is invoked
	// if no credentials provided, or the user can not be found.
	// For API key authentication the response code is 401.
	//
	// Note that the handler does not actually redirect but
	// only responds with a distinct code: that's done for
	// testing purposes only.
	redirect := func(w http.ResponseWriter, r *http.Request) {
		logger.WithField("url", r.URL).Debug("redirecting")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}

	r := router.New(
		logger,
		redirect,
		mux.NewRouter(),
		services)

	if services.AuthService != nil {
		r.Use(api.AuthMiddleware(logger, redirect, r.AuthService))
	}

	r.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(rcp(r.Context()))
			next(w, r)
		}
	})

	r.RegisterHandlers()
	return r
}

// withRequest returns a function than performs an HTTP request
// with the body specified, and validates the response code and body.
//
// Request and response body ("in" and "out", correspondingly) are
// specified as a file name relative to the "testdata" directory.
// Either of "in" and "out" can be an empty string.
func withRequest(method, url string) func(code int, in, out string) {
	return func(code int, in, out string) {
		var reqBody io.Reader
		if in != "" {
			reqBody = readFile(in)
		}
		req, err := http.NewRequest(method, url, reqBody)
		Expect(err).ToNot(HaveOccurred())
		response, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.StatusCode).To(Equal(code))
		if out == "" {
			Expect(readBody(response).String()).To(BeEmpty())
			return
		}
		// It may also make sense to accept the response as a template
		// and render non-deterministic values.
		Expect(readBody(response)).To(MatchJSON(readFile(out)))
	}
}

func readFile(path string) *bytes.Buffer {
	b, err := os.ReadFile("testdata/" + path)
	Expect(err).ToNot(HaveOccurred())
	return bytes.NewBuffer(b)
}

func readBody(r *http.Response) *bytes.Buffer {
	b, err := io.ReadAll(r.Body)
	Expect(err).ToNot(HaveOccurred())
	_ = r.Body.Close()
	return bytes.NewBuffer(b)
}
