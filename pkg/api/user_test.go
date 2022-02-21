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

	"github.com/pyroscope-io/pyroscope/pkg/api/mocks"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
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
		method, url string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		m = mocks.NewMockUserService(ctrl)
		server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
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
			url = server.URL + "/users"

			expectedParams = model.CreateUserParams{
				Name:     "johndoe",
				Email:    model.String("john@example.com"),
				FullName: model.String("John Doe"),
				Password: "qwerty",
				Role:     model.ReadOnlyRole,
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
				CreatedAt:         now,
				UpdatedAt:         now,
			}
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

				expectResponse(newRequest(method, url,
					"user/create_request.json"),
					"user/create_response.json",
					http.StatusCreated)
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

				expectResponse(newRequest(method, url,
					"user/create_request_wo_full_name.json"),
					"user/create_response_wo_full_name.json",
					http.StatusCreated)
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

				expectResponse(newRequest(method, url,
					"user/create_request.json"),
					"user/create_response_email_exists.json",
					http.StatusBadRequest)
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

				expectResponse(newRequest(method, url,
					"user/create_request.json"),
					"user/create_response_user_name_exists.json",
					http.StatusBadRequest)
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

				expectResponse(newRequest(method, url,
					"request_empty_object.json"),
					"user/create_response_invalid.json",
					http.StatusBadRequest)
			})
		})

		Context("when request body malformed", func() {
			It("returns error and does not call user service", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Times(0)

				expectResponse(newRequest(method, url,
					"request_malformed_json"),
					"response_malformed_request_body.json",
					http.StatusBadRequest)
			})
		})

		Context("when request has empty body", func() {
			It("returns error and does not call user service", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Times(0)

				expectResponse(newRequest(method, url,
					""), // No request body.
					"response_empty_request_body.json",
					http.StatusBadRequest)
			})
		})
	})
})
