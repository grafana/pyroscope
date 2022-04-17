package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

func TestAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Suite")
}

// requestContextProvider wraps incoming request context.
// Mainly used to inject auth info; use defaultUserCtx if not sure.
type requestContextProvider func(context.Context) context.Context

var (
	defaultUserCtx = ctxWithUser(&model.User{ID: 1, Role: model.AdminRole})
	defaultReqCtx  = func(ctx context.Context) context.Context { return ctx }
)

func ctxWithUser(u *model.User) requestContextProvider {
	return func(ctx context.Context) context.Context {
		return model.WithUser(ctx, *u)
	}
}

func ctxWithAPIKey(k *model.APIKey) requestContextProvider {
	return func(ctx context.Context) context.Context {
		return model.WithAPIKey(ctx, *k)
	}
}

// newTestRouter initializes http router for testing purposes.
// It was decided to test the whole flow of the request handling,
// including routing, authentication, and authorization.
func newTestRouter(rcp requestContextProvider, services router.Services) *router.Router {
	// For backward compatibility, the redirect handler is invoked
	// if no credentials provided, or the user can not be found.
	// For API key authentication the response code is 401.
	//
	// Note that the handler does not actually redirect but
	// only responds with a distinct code: that's done for
	// testing purposes only.
	redirect := func(w http.ResponseWriter, r *http.Request) {
		services.Logger.WithField("url", r.URL).Debug("redirecting")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}

	r := router.New(
		mux.NewRouter(),
		services)

	if services.AuthService != nil {
		r.Use(api.AuthMiddleware(services.Logger, redirect, r.AuthService, httputils.NewDefaultHelper(services.Logger)))
	}

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(rcp(r.Context())))
		})
	})

	r.RegisterHandlers()
	return r
}

// newRequest creates an HTTP request with the body specified specified
// as a file name relative to the "testdata".
func newRequest(method, url, body string) *http.Request {
	var reqBody io.Reader
	if body != "" {
		reqBody = readFile(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	Expect(err).ToNot(HaveOccurred())
	return req
}

// expectResponse performs an HTTP request and validates the response
// code and body which is specified as a file name relative to the "testdata".
func expectResponse(req *http.Request, body string, code int) {
	response, err := http.DefaultClient.Do(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(response).ToNot(BeNil())
	Expect(response.StatusCode).To(Equal(code))
	if body == "" {
		Expect(readBody(response).String()).To(BeEmpty())
		return
	}
	// It may also make sense to accept the response as a template
	// and render non-deterministic values.
	Expect(readBody(response)).To(MatchJSON(readFile(body)))
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
