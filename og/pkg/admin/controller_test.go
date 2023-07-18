// TODO: move most of these tests to pkg/api
package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type mockStorage struct {
	getAppsResult []appmetadata.ApplicationMetadata
	deleteResult  error
}

func (m mockStorage) List(ctx context.Context) ([]appmetadata.ApplicationMetadata, error) {
	return m.getAppsResult, nil
}

func (m mockStorage) Delete(ctx context.Context, appname string) error {
	return m.deleteResult
}

type mockUserService struct{}

func (mockUserService) UpdateUserByName(context.Context, string, model.UpdateUserParams) (model.User, error) {
	return model.User{}, nil
}

type mockStorageService struct{}

func (mockStorageService) Cleanup(context.Context) error {
	return nil
}

var _ = Describe("controller", func() {
	Describe("/v1/apps", func() {
		var svr *admin.Server
		var response *httptest.ResponseRecorder
		var appSvc admin.ApplicationListerAndDeleter

		// create a server
		BeforeEach(func() {
			appSvc = mockStorage{
				getAppsResult: []appmetadata.ApplicationMetadata{
					{FQName: "app1"},
					{FQName: "app2"},
				},
				deleteResult: nil,
			}
		})

		JustBeforeEach(func() {
			// create a null logger, since we aren't interested
			logger, _ := test.NewNullLogger()

			ctrl := admin.NewController(logger, appSvc, mockUserService{}, mockStorageService{})
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

					body, err := io.ReadAll(response.Body)
					Expect(err).To(BeNil())

					Expect(response.Code).To(Equal(http.StatusOK))
					Expect(string(body)).To(Equal(`[{"name":"app1"},{"name":"app2"}]
`))
				})

			})
		})

		Describe("DELETE", func() {
			Context("when everything is right", func() {
				It("returns StatusOK", func() {
					// we are kinda robbing here
					// since the server and client are defined in the same package
					payload := api.DeleteAppInput{Name: "myapp"}
					marshalledPayload, err := json.Marshal(payload)
					request, err := http.NewRequest(http.MethodDelete, "/v1/apps", bytes.NewBuffer(marshalledPayload))
					Expect(err).ToNot(HaveOccurred())

					svr.Handler.ServeHTTP(response, request)
					Expect(response.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when there's an error", func() {
				Context("with the payload", func() {
					BeforeEach(func() {
						appSvc = mockStorage{
							deleteResult: model.ValidationError{},
						}
					})

					It("returns BadRequest", func() {
						request, err := http.NewRequest(http.MethodDelete, "/v1/apps", bytes.NewBuffer([]byte(``)))
						Expect(err).ToNot(HaveOccurred())

						svr.Handler.ServeHTTP(response, request)
						Expect(response.Code).To(Equal(http.StatusBadRequest))
					})
				})

				Context("with the server", func() {
					BeforeEach(func() {
						appSvc = mockStorage{
							deleteResult: fmt.Errorf("error"),
						}
					})

					It("returns InternalServerError", func() {
						// we are kinda robbing here
						// since the server and client are defined in the same package
						payload := api.DeleteAppInput{Name: "myapp"}
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
