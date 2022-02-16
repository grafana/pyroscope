package api_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/mocks"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("AuthMiddleware", func() {
	defer GinkgoRecover()

	var (
		// Mocks setup.
		ctrl            *gomock.Controller
		server          *httptest.Server
		authServiceMock *mocks.MockAuthService

		// The service is a sample target.
		apiKeyServiceMock *mocks.MockAPIKeyService
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		authServiceMock = mocks.NewMockAuthService(ctrl)
		apiKeyServiceMock = mocks.NewMockAPIKeyService(ctrl)
		server = httptest.NewServer(newTestRouter(defaultReqCtx, router.Services{
			Logger:        logrus.StandardLogger(),
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
			expectedAPIKey model.APIKey
			expectedUser   model.User
			expectedToken  string
		)

		BeforeEach(func() {
			expectedToken = "some-token"
			expectedAPIKey = model.APIKey{
				Name: "test-api-key",
				Role: model.AdminRole,
			}
			expectedUser = model.User{
				Name: "test-user",
				Role: model.AdminRole,
			}
		})

		Context("when request has a valid API key in the header", func() {
			It("authenticates request", func() {
				authServiceMock.EXPECT().
					APIKeyFromToken(gomock.Any(), expectedToken).
					Return(expectedAPIKey, nil).
					Times(1)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).
					Times(1)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.Header.Set("Authorization", "Bearer "+expectedToken)
				expectResponse(req,
					"response_empty_array.json",
					http.StatusOK)
			})
		})

		Context("when request has an invalid API key in the header", func() {
			It("returns status code Unauthorized", func() {
				authServiceMock.EXPECT().
					APIKeyFromToken(gomock.Any(), expectedToken).
					Return(expectedAPIKey, model.ErrAPIKeyNotFound).
					Times(1)

				authServiceMock.EXPECT().
					UserFromJWTToken(gomock.Any(), expectedToken).
					Times(0)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).
					Times(0)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.Header.Set("Authorization", "Bearer "+expectedToken)
				expectResponse(req,
					"response_invalid_credentials.json",
					http.StatusUnauthorized)
			})
		})

		Context("when request has a valid user token in the cookies", func() {
			It("authenticates request", func() {
				authServiceMock.EXPECT().
					UserFromJWTToken(gomock.Any(), expectedToken).
					Return(expectedUser, nil).
					Times(1)

				authServiceMock.EXPECT().
					APIKeyFromToken(gomock.Any(), expectedToken).
					Times(0)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).
					Times(1)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.AddCookie(&http.Cookie{Name: api.JWTCookieName, Value: expectedToken})
				expectResponse(req,
					"response_empty_array.json",
					http.StatusOK)
			})
		})

		Context("when user token is invalid or can not be found", func() {
			It("redirects request", func() {
				authServiceMock.EXPECT().
					UserFromJWTToken(gomock.Any(), expectedToken).
					Return(expectedUser, model.ErrUserNotFound).
					Times(1)

				authServiceMock.EXPECT().
					APIKeyFromToken(gomock.Any(), expectedToken).
					Times(0)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).
					Times(0)

				req := newRequest(http.MethodGet, server.URL+"/keys", "")
				req.AddCookie(&http.Cookie{Name: api.JWTCookieName, Value: expectedToken})
				expectResponse(req,
					"", // Empty response body.
					http.StatusTemporaryRedirect)
			})
		})

		Context("when credentials are not provided", func() {
			It("redirects request", func() {
				authServiceMock.EXPECT().
					APIKeyFromToken(gomock.Any(), expectedToken).
					Return(expectedAPIKey, nil).
					Times(0)

				authServiceMock.EXPECT().
					UserFromJWTToken(gomock.Any(), expectedToken).
					Return(expectedUser, model.ErrUserNotFound).
					Times(0)

				apiKeyServiceMock.EXPECT().
					GetAllAPIKeys(gomock.Any()).
					Times(0)

				expectResponse(newRequest(http.MethodGet, server.URL+"/keys",
					""), // Empty request body.
					"", // Empty response.
					http.StatusTemporaryRedirect)
			})
		})
	})
})
