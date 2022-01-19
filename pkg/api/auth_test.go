package api_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"

	"github.com/pyroscope-io/pyroscope/pkg/api/mocks"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("AuthMiddleware", func() {
	defer GinkgoRecover()

	var (
		// Mocks setup.
		ctrl              *gomock.Controller
		server            *httptest.Server
		authServiceMock   *mocks.MockAuthService
		apiKeyServiceMock *mocks.MockAPIKeyService
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		authServiceMock = mocks.NewMockAuthService(ctrl)
		apiKeyServiceMock = mocks.NewMockAPIKeyService(ctrl)
		server = httptest.NewServer(newTestRouter(defaultReqCtx, router.Services{
			AuthService:   authServiceMock,
			APIKeyService: apiKeyServiceMock,
		}))
	})

	AfterEach(func() {
		ctrl.Finish()
		server.Close()
	})

	Describe("request authentication", func() {
		var (
			// API key and JWT token string returned by mocked service.
			expectedAPIKey   model.TokenAPIKey
			expectedJWTToken string
		)

		BeforeEach(func() {
			expectedJWTToken = "some-jwt-token"
			expectedAPIKey = model.TokenAPIKey{
				Name: "test-api-key",
				Role: model.AdminRole,
			}
		})

		Context("when request is complete and valid", func() {
			It("authenticates request", func() {
				authServiceMock.EXPECT().
					APIKeyFromJWTToken(gomock.Any(), expectedJWTToken).
					Return(expectedAPIKey, nil)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).Times(1)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.Header.Set("Authorization", "Bearer "+expectedJWTToken)
				expectResponse(req,
					"response_empty_array.json",
					http.StatusOK)
			})
		})

		Context("when API key is invalid or can not be found", func() {
			// JWT verification is out of scope of the service,
			// see service.JWTTokenService.
			It("returns status code Unauthorized", func() {
				authServiceMock.EXPECT().
					APIKeyFromJWTToken(gomock.Any(), expectedJWTToken).
					Return(expectedAPIKey, model.ErrAPIKeyNotFound).Times(1)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).Times(0)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.Header.Set("Authorization", "Bearer "+expectedJWTToken)
				expectResponse(req,
					"response_invalid_credentials.json",
					http.StatusUnauthorized)
			})
		})

		// Make sure redirection logic was preserved.
		Context("when credentials are not provided", func() {
			It("redirects request", func() {
				authServiceMock.EXPECT().
					APIKeyFromJWTToken(gomock.Any(), expectedJWTToken).
					Return(expectedAPIKey, nil).Times(0)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).Times(0)

				expectResponse(newRequest(http.MethodGet, server.URL+"/keys",
					""), // Empty request body.
					"", // Empty response.
					http.StatusTemporaryRedirect)
			})
		})
	})
})
