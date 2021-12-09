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
	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/api"
	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

var _ = Describe("UserHandler", func() {
	defer GinkgoRecover()

	var (
		ctrl   *gomock.Controller
		server *httptest.Server
		m      *MockUserService
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		m = NewMockUserService(ctrl)
		server = httptest.NewServer(api.Router(&api.Services{
			UserService: m,
		}))
	})

	AfterEach(func() {
		ctrl.Finish()
		server.Close()
	})

	Describe("create user", func() {
		checkResponse := func(code int, in, out string) {
			requestBody := readFile(in)
			response, err := http.DefaultClient.Post(server.URL+"/api/users", "", requestBody)
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			Expect(response.StatusCode).To(Equal(code))
			expectedResponse := readFile(out)
			Expect(readBody(response)).To(MatchJSON(expectedResponse))
		}

		Context("when request is valid", func() {
			var (
				params model.CreateUserParams
				user   model.User
			)

			BeforeEach(func() {
				// Params passed to the mocked user service.
				params = model.CreateUserParams{
					FullName: model.String("John Doe"),
					Email:    "john@example.com",
					Password: "qwerty",
					Role:     model.ViewerRole,
				}

				// User returned by mocked service.
				now := time.Date(2021, 12, 10, 4, 14, 0, 0, time.UTC)
				user = model.User{
					Model: gorm.Model{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
						DeletedAt: gorm.DeletedAt{},
					},
					FullName:          *params.FullName,
					Email:             params.Email,
					PasswordHash:      model.MustPasswordHash(params.Password),
					Role:              params.Role,
					PasswordChangedAt: now,
				}
			})

			It("returns expected response", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(user, nil).
					Do(func(_ context.Context, user model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(user).To(Equal(params))
					})

				checkResponse(http.StatusCreated,
					"user_create_request.json",
					"user_create_response.json")
			})
		})

		Context("when request does not meet requirements", func() {
			It("returns validation errors", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Return(model.User{}, &multierror.Error{Errors: []error{
						model.ErrUserPasswordEmpty,
						model.ErrUserNameEmpty,
						model.ErrRoleUnknown,
						model.ErrUserEmailInvalid,
					}}).
					Do(func(_ context.Context, user model.CreateUserParams) {
						defer GinkgoRecover()
						Expect(user).To(BeZero())
					})

				checkResponse(http.StatusBadRequest,
					"user_create_request_invalid.json",
					"user_create_response_invalid.json")
			})
		})

		Context("when request body malformed", func() {
			It("returns error and does not call user service", func() {
				m.EXPECT().
					CreateUser(gomock.Any(), gomock.Any()).
					Times(0)

				checkResponse(http.StatusBadRequest,
					"user_create_request.malformed",
					"user_create_response_malformed.json")
			})
		})
	})
})
