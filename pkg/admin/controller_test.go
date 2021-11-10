package admin_test

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

type mockAppsGetter struct{}

func (mockAppsGetter) GetAppNames() []string {
	return []string{"app1", "app2"}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var _ = Describe("controller", func() {
	// TODO
	// since this talks with socket and stuff
	// this can become an integration test
	Context("/v1/apps", func() {
		var (
			httpC http.Client

			socketAddr string
		)

		BeforeEach(func() {
			file, err := ioutil.TempFile("", "pyroscope.sock")
			must(err)
			socketAddr = file.Name()

			cfg := admin.Config{
				SocketAddr: socketAddr,
			}
			svc := admin.NewService(mockAppsGetter{})
			ctrl, err := admin.NewController(cfg, svc)
			must(err)

			go func() {
				err := ctrl.Start()
				if err != nil {
					panic(err)
				}
			}()

			httpC = newHttpClient(socketAddr)
			// TODO
			// for some reason it takes some time until server is ready
			// we could add retries?
			time.Sleep(time.Millisecond * 10)
		})

		AfterEach(func() {
			os.Remove(socketAddr)
		})

		It("returns app names", func() {
			req, err := httpC.Get("http://dummy/v1/apps")
			Expect(err).To(BeNil())

			body, err := ioutil.ReadAll(req.Body)
			Expect(err).To(BeNil())

			Expect(string(body)).To(Equal(`["app1","app2"]
`))
		})

		It("only accepts GET requests", func() {

			Expect(true).To(Equal(true))
		})
	})

	Context("/v1/apps", func() {
		var svr *admin.AdminServer
		var response *httptest.ResponseRecorder

		// create a server
		BeforeEach(func() {
			cfg := admin.Config{SocketAddr: "foo"}

			svc := admin.NewService(mockAppsGetter{})
			server, err := admin.NewServer(cfg, svc)

			must(err)
			svr = server
			response = httptest.NewRecorder()
		})

		It("returns list of apps", func() {
			request, _ := http.NewRequest(http.MethodGet, "/v1/apps", nil)

			svr.Handler.ServeHTTP(response, request)

			body, err := ioutil.ReadAll(response.Body)
			Expect(err).To(BeNil())

			Expect(response.Code).To(Equal(200))
			Expect(string(body)).To(Equal(`["app1","app2"]
`))
		})

		DescribeTable("Non GET requests return 405 (method not allowed)",
			func(method string) {
				request, _ := http.NewRequest(method, "/v1/apps", nil)
				svr.Handler.ServeHTTP(response, request)
				Expect(response.Code).To(Equal(405))
			},
			Entry("POST", http.MethodPost),
			Entry("PUT", http.MethodPost),
			Entry("DELETE", http.MethodPost),
			Entry("NON_VALID_METHOD", http.MethodPost),
		)
	})
})

func newHttpClient(socketAddr string) http.Client {
	return http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}
}
