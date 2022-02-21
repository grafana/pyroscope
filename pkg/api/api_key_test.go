package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/api/mocks"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("APIKeyHandler", func() {
	defer GinkgoRecover()

	var (
		// Mocks setup.
		ctrl   *gomock.Controller
		server *httptest.Server
		m      *mocks.MockAPIKeyService

		// Default configuration for all scenarios.
		method, url string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		m = mocks.NewMockAPIKeyService(ctrl)
		server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
			APIKeyService: m,
		}))
	})

	AfterEach(func() {
		ctrl.Finish()
		server.Close()
	})

	Describe("create API key", func() {
		var (
			// Expected params passed to the mocked API key service.
			expectedParams model.CreateAPIKeyParams
			// API key and JWT token string returned by mocked service.
			expectedAPIKey model.APIKey
			expectedSecret string
		)

		BeforeEach(func() {
			// Defaults for all "create API key" scenarios.
			method = http.MethodPost
			url = server.URL + "/keys"

			// Note that the actual ExpiresAt is populated during the handler execution
			// and it is relative to time.Now(). Therefore use this mather to evaluate
			// the actual expiration time: BeTemporally("~", time.Now(), time.Minute).
			now := time.Date(2021, 12, 10, 4, 14, 0, 0, time.UTC)
			expiresAt := now.Add(time.Minute)

			expectedSecret = "secret-string"
			expectedParams = model.CreateAPIKeyParams{
				Name:      "some-api-key",
				Role:      model.ReadOnlyRole,
				ExpiresAt: &expiresAt,
			}

			expectedAPIKey = model.APIKey{
				ID:         1,
				Name:       expectedParams.Name,
				Role:       expectedParams.Role,
				ExpiresAt:  expectedParams.ExpiresAt,
				LastSeenAt: nil,
				CreatedAt:  now,
			}
		})

		Context("when request is complete and valid", func() {
			It("responds with created API key", func() {
				m.EXPECT().
					CreateAPIKey(gomock.Any(), gomock.Any()).
					Return(expectedAPIKey, expectedSecret, nil).
					Do(func(_ context.Context, actual model.CreateAPIKeyParams) {
						defer GinkgoRecover()
						Expect(*actual.ExpiresAt).To(BeTemporally("~", time.Now(), time.Minute))
						Expect(actual.Name).To(Equal(expectedParams.Name))
						Expect(actual.Role).To(Equal(expectedParams.Role))
					})

				expectResponse(newRequest(method, url,
					"api_key/create_request.json"),
					"api_key/create_response.json",
					http.StatusCreated)
			})
		})

		Context("when api key ttl is not specified", func() {
			It("responds with created API key", func() {
				expectedParams.ExpiresAt = nil
				expectedAPIKey.ExpiresAt = nil

				m.EXPECT().
					CreateAPIKey(gomock.Any(), expectedParams).
					Return(expectedAPIKey, expectedSecret, nil)

				expectResponse(newRequest(method, url,
					"api_key/create_request_wo_ttl.json"),
					"api_key/create_response_wo_ttl.json",
					http.StatusCreated)
			})
		})

		Context("when the request does not meet requirements", func() {
			It("returns validation errors", func() {
				m.EXPECT().
					CreateAPIKey(gomock.Any(), model.CreateAPIKeyParams{}).
					Return(model.APIKey{}, "", &multierror.Error{Errors: []error{
						model.ErrAPIKeyNameEmpty,
						model.ErrRoleUnknown,
					}})

				expectResponse(newRequest(method, url,
					"request_empty_object.json"),
					"api_key/create_response_invalid.json",
					http.StatusBadRequest)
			})
		})
	})
})
