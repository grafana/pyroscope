package admin_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"

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
	Context("/v1/apps", func() {
		var svr *admin.Server
		var response *httptest.ResponseRecorder

		// create a server
		BeforeEach(func() {
			// create a null logger, since we aren't interested
			logger, _ := test.NewNullLogger()

			cfg := admin.Config{SocketAddr: "foo", Log: logger}

			svc := admin.NewService(mockAppsGetter{})
			ctrl := admin.NewController(logger, svc)
			httpServer := &admin.UdsHTTPServer{}
			server, err := admin.NewServer(cfg, ctrl, httpServer)

			must(err)
			svr = server
			response = httptest.NewRecorder()
		})

		It("returns list of apps", func() {
			request, _ := http.NewRequest(http.MethodGet, "/v1/apps", nil)

			svr.Mux.ServeHTTP(response, request)

			body, err := ioutil.ReadAll(response.Body)
			Expect(err).To(BeNil())

			Expect(response.Code).To(Equal(200))
			Expect(string(body)).To(Equal(`["app1","app2"]
`))
		})

		DescribeTable("Non GET requests return 405 (method not allowed)",
			func(method string) {
				request, _ := http.NewRequest(method, "/v1/apps", nil)
				svr.Mux.ServeHTTP(response, request)
				Expect(response.Code).To(Equal(405))
			},
			Entry("POST", http.MethodPost),
			Entry("PUT", http.MethodPost),
			Entry("DELETE", http.MethodPost),
			Entry("NON_VALID_METHOD", http.MethodPost),
		)
	})
})
