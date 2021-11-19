package admin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type mockStorage struct {
	getAppNamesResult []string
	deleteResult      error
}

func (m mockStorage) GetAppNames() []string {
	return m.getAppNamesResult
}

func (m mockStorage) Delete(di *storage.DeleteInput) error {
	return m.deleteResult
}

var _ = Describe("controller", func() {
	Describe("/v1/apps", func() {
		var svr *admin.Server
		var response *httptest.ResponseRecorder
		var storage admin.Storage

		// create a server
		BeforeEach(func() {
			storage = mockStorage{
				getAppNamesResult: []string{"app1", "app2"},
				deleteResult:      nil,
			}
		})

		JustBeforeEach(func() {
			// create a null logger, since we aren't interested
			logger, _ := test.NewNullLogger()

			svc := admin.NewService(storage)
			ctrl := admin.NewController(logger, svc)
			httpServer := &admin.UdsHTTPServer{}
			server, err := admin.NewServer(logger, ctrl, httpServer)

			Expect(err).ToNot(HaveOccurred())
			svr = server
			response = httptest.NewRecorder()
		})

		Describe("GET", func() {
			Context("when everything is right", func() {
				It("returns list of apps successfully", func() {
					request, err := http.NewRequest(http.MethodGet, "/v1/apps", nil)
					Expect(err).ToNot(HaveOccurred())

					svr.Handler.ServeHTTP(response, request)

					body, err := ioutil.ReadAll(response.Body)
					Expect(err).To(BeNil())

					Expect(response.Code).To(Equal(http.StatusOK))
					Expect(string(body)).To(Equal(`["app1","app2"]
`))
				})

			})
		})

		Describe("DELETE", func() {
			Context("when everything is right", func() {
				It("returns StatusOK", func() {
					// we are kinda robbing here
					// since the server and client are defined in the same package
					payload := admin.DeleteAppInput{Name: "myapp"}
					marshalledPayload, err := json.Marshal(payload)
					request, err := http.NewRequest(http.MethodDelete, "/v1/apps", bytes.NewBuffer(marshalledPayload))
					Expect(err).ToNot(HaveOccurred())

					svr.Handler.ServeHTTP(response, request)
					Expect(response.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when there's an error", func() {
				Context("with the payload", func() {
					It("returns BadRequest", func() {
						request, err := http.NewRequest(http.MethodDelete, "/v1/apps", bytes.NewBuffer([]byte(``)))
						Expect(err).ToNot(HaveOccurred())

						svr.Handler.ServeHTTP(response, request)
						Expect(response.Code).To(Equal(http.StatusBadRequest))
					})
				})

				Context("with the server", func() {
					BeforeEach(func() {
						storage = mockStorage{
							deleteResult: fmt.Errorf("error"),
						}
					})

					It("returns InternalServerError", func() {
						// we are kinda robbing here
						// since the server and client are defined in the same package
						payload := admin.DeleteAppInput{Name: "myapp"}
						marshalledPayload, err := json.Marshal(payload)
						request, err := http.NewRequest(http.MethodDelete, "/v1/apps", bytes.NewBuffer(marshalledPayload))
						Expect(err).ToNot(HaveOccurred())

						svr.Handler.ServeHTTP(response, request)
						Expect(response.Code).To(Equal(http.StatusInternalServerError))
					})
				})
			})
		})

		DescribeTable("Non supported requests return 405 (method not allowed)",
			func(method string) {
				request, _ := http.NewRequest(method, "/v1/apps", nil)
				svr.Handler.ServeHTTP(response, request)
				Expect(response.Code).To(Equal(405))
			},
			Entry("POST", http.MethodPost),
			Entry("PUT", http.MethodPost),
			Entry("NON_VALID_METHOD", http.MethodPost),
		)
	})
})
