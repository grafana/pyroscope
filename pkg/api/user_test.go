package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/api/mocks"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("UserHandler", func() {
	defer GinkgoRecover()

	var (
		// Mocks setup.
		ctrl   *gomock.Controller
		server *httptest.Server
		m      *mocks.MockUserService

		// Default configuration for all scenarios.
		method, url    string
		expectResponse func(code int, in, out string)
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		m = mocks.NewMockUserService(ctrl)
		server = httptest.NewServer(api.Router(&api.Services{
			UserService: m,
		}))
	})

	AfterEach(func() {
		ctrl.Finish()
		server.Close()
	})

	Describe("create user", func() {
		var (
			// Expected params passed to the mocked user service.
			expectedParams model.CreateUserParams
			// User returned by mocked service.
			expectedUser model.User
		)

		BeforeEach(func() {
			// Defaults for all "create user" scenarios.
			method = http.MethodPost
			url = server.URL + "/api/users"

			expectedParams = model.CreateUserParams{
				Name:     "johndoe",
				Email:    "john@example.com",
				FullName: model.String("John Doe"),
				Password: "qwerty",
				Role:     model.ViewerRole,
			}

			now := time.Date(2021, 12, 10, 4, 14, 0, 0, time.UTC)
			expectedUser = model.User{
				ID:                1,
				Name:              expectedParams.Name,
				Email:             expectedParams.Email,
				FullName:          expectedParams.FullName,
				Role:              expectedParams.Role,
				PasswordHash:      model.MustPasswordHash(expectedParams.Password),
				PasswordChangedAt: now,
				LastSeenAt:        nil,
				CreatedAt:         now,
				UpdatedAt:         now,
				DeletedAt:         nil,
			}
		})

		JustBeforeEach(func() {
			// The function is generated just before It, and should be only
			// called after the mock is set up with mock.EXPECT call.
			expectResponse = withRequest(method, url)
		})

		Context("when request is complete and valid", func() {
			It("responds with created user", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(expectedUser, nil).
					Do(func(_ context.Context, actual model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(actual).To(Equal(expectedParams))
					})

				expectResponse(http.StatusCreated,
					"user_create_request.json",
					"user_create_response.json")
			})
		})

		Context("when user full name is not specified", func() {
			It("responds with created user", func() {
				expectedParams.FullName = nil
				expectedUser.FullName = nil

				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(expectedUser, nil).
					Do(func(_ context.Context, actual model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(actual).To(Equal(expectedParams))
					})

				expectResponse(http.StatusCreated,
					"user_create_request_wo_full_name.json",
					"user_create_response_wo_full_name.json")
			})
		})

		Context("when email already exists", func() {
			It("returns ErrUserEmailExists error", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(model.User{}, &multierror.Error{Errors: []error{
						model.ErrUserEmailExists,
					}}).
					Do(func(_ context.Context, actual model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(actual).To(Equal(expectedParams))
					})

				expectResponse(http.StatusBadRequest,
					"user_create_request.json",
					"user_create_response_email_exists.json")
			})
		})

		Context("when user name already exists", func() {
			It("returns ErrUserNameExists error", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(model.User{}, &multierror.Error{Errors: []error{
						model.ErrUserNameExists,
					}}).
					Do(func(_ context.Context, actual model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(actual).To(Equal(expectedParams))
					})

				expectResponse(http.StatusBadRequest,
					"user_create_request.json",
					"user_create_response_user_name_exists.json")
			})
		})

		Context("when request does not meet requirements", func() {
			It("returns validation errors", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(model.User{}, &multierror.Error{Errors: []error{
						model.ErrUserNameEmpty,
						model.ErrUserEmailInvalid,
						model.ErrUserPasswordEmpty,
						model.ErrRoleUnknown,
					}}).
					Do(func(_ context.Context, user model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(user).To(BeZero())
					})

				expectResponse(http.StatusBadRequest,
					"request_empty_object.json",
					"user_create_response_invalid.json")
			})
		})

		Context("when request body malformed", func() {
			It("returns error and does not call user service", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Times(0)

				expectResponse(http.StatusBadRequest,
					"request_malformed_json",
					"response_malformed_request_body.json")
			})
		})

		Context("when request has empty body", func() {
			It("returns error and does not call user service", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Times(0)

				expectResponse(http.StatusBadRequest,
					"", // No request body.
					"response_empty_request_body.json")
			})
		})
	})
})
