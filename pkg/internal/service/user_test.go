package service_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
	"github.com/pyroscope-io/pyroscope/pkg/internal/service"
)

var _ = Describe("UserService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	var svc service.UserService
	BeforeEach(func() {
		svc = service.NewUserService(s.DB())
	})

	Describe("user creation", func() {
		var (
			params = testCreateUserParams()[0]
			user   model.User
			err    error
		)

		JustBeforeEach(func() {
			user, err = svc.CreateUser(context.Background(), params)
		})

		Context("when a new user created", func() {
			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should populate the fields correctly", func() {
				expectUserMatches(user, params)
			})

			It("creates valid password hash", func() {
				err = model.VerifyPassword(user.PasswordHash, params.Password)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when user email is already in use", func() {
			BeforeEach(func() {
				_, err = svc.CreateUser(context.Background(), params)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns validation error", func() {
				Expect(model.IsValidationError(err)).To(BeTrue())
			})
		})

		Context("when user is invalid", func() {
			BeforeEach(func() {
				params = model.CreateUserParams{}
			})

			It("returns validation error", func() {
				Expect(model.IsValidationError(err)).To(BeTrue())
			})
		})
	})

	Describe("user retrieval", func() {
		var (
			params = testCreateUserParams()[0]
			user   model.User
			err    error
		)

		Context("when an existing user is queried", func() {
			BeforeEach(func() {
				user, err = svc.CreateUser(context.Background(), params)
				Expect(err).ToNot(HaveOccurred())
			})

			It("can be found", func() {
				By("id", func() {
					user, err = svc.FindUserByID(context.Background(), user.ID)
					Expect(err).ToNot(HaveOccurred())
					expectUserMatches(user, params)
				})

				By("email", func() {
					user, err = svc.FindUserByEmail(context.Background(), params.Email)
					Expect(err).ToNot(HaveOccurred())
					expectUserMatches(user, params)
				})
			})
		})

		Context("when a non-existing user is queried", func() {
			It("returns ErrUserNotFound error of NotFoundError type", func() {
				By("id", func() {
					_, err = svc.FindUserByID(context.Background(), 0)
					Expect(err).To(MatchError(model.ErrUserNotFound))
				})

				By("email", func() {
					_, err = svc.FindUserByEmail(context.Background(), params.Email)
					Expect(err).To(MatchError(model.ErrUserNotFound))
				})
			})
		})
	})

	Describe("users retrieval", func() {
		var (
			params = testCreateUserParams()
			users  []model.User
			err    error
		)

		JustBeforeEach(func() {
			users, err = svc.GetAllUsers(context.Background())
		})

		Context("when all users are queried", func() {
			BeforeEach(func() {
				for _, user := range params {
					_, err = svc.CreateUser(context.Background(), user)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns all users", func() {
				user1, err := svc.FindUserByEmail(context.Background(), params[0].Email)
				Expect(err).ToNot(HaveOccurred())
				user2, err := svc.FindUserByEmail(context.Background(), params[1].Email)
				Expect(err).ToNot(HaveOccurred())
				Expect(users).To(ConsistOf(user1, user2))
			})
		})

		Context("when no users exist", func() {
			It("returns no error", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(users).To(BeEmpty())
			})
		})
	})

	Describe("user update", func() {
		var (
			params  = testCreateUserParams()
			update  model.UpdateUserParams
			user    model.User
			updated model.User
			err     error
		)

		JustBeforeEach(func() {
			updated, err = svc.UpdateUserByID(context.Background(), user.ID, update)
		})

		Context("when no parameters specified", func() {
			BeforeEach(func() {
				user, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("does not change user", func() {
				updated, err = svc.FindUserByID(context.Background(), user.ID)
				Expect(err).ToNot(HaveOccurred())
				expectUserMatches(updated, params[0])
			})
		})

		Context("when parameters provided", func() {
			BeforeEach(func() {
				user, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
				update = model.UpdateUserParams{
					FullName: model.String("Jonny"),
					Email:    model.String("admin@local.domain"),
					Password: model.String("qwerty")}.
					SetRole(model.AdminRole).
					SetIsDisabled(true)
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("updates user fields", func() {
				updated, err = svc.FindUserByID(context.Background(), user.ID)
				Expect(err).ToNot(HaveOccurred())
				Expect(updated.FullName).To(Equal(update.FullName))
				Expect(updated.Email).To(Equal(*update.Email))
				Expect(updated.Role).To(Equal(*update.Role))
				Expect(*updated.IsDisabled).To(BeTrue())
				Expect(updated.CreatedAt).ToNot(BeZero())
				Expect(updated.UpdatedAt).ToNot(BeZero())
				Expect(updated.UpdatedAt).ToNot(Equal(updated.CreatedAt))
				Expect(updated.PasswordHash).ToNot(Equal(user.PasswordHash))
				Expect(updated.PasswordChangedAt).ToNot(BeZero())
				Expect(updated.PasswordChangedAt).ToNot(Equal(user.PasswordChangedAt))
			})
		})

		Context("when parameters invalid", func() {
			BeforeEach(func() {
				user, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
				update = model.UpdateUserParams{
					FullName: model.String(""),
					Email:    model.String(""),
					Password: model.String("")}.
					SetRole(model.InvalidRole)
			})

			It("returns ValidationError", func() {
				Expect(model.IsValidationError(err)).To(BeTrue())
				Expect(err).To(MatchError(model.ErrUserNameEmpty))
				Expect(err).To(MatchError(model.ErrUserEmailInvalid))
				Expect(err).To(MatchError(model.ErrRoleUnknown))
				Expect(err).To(MatchError(model.ErrUserPasswordEmpty))
			})
		})

		Context("when user is disabled", func() {
			BeforeEach(func() {
				user, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
				update = model.UpdateUserParams{}.SetIsDisabled(true)
			})

			It("can be enabled again", func() {
				Expect(err).ToNot(HaveOccurred())

				update = model.UpdateUserParams{}.SetIsDisabled(false)
				_, err = svc.UpdateUserByID(context.Background(), user.ID, update)
				Expect(err).ToNot(HaveOccurred())

				updated, err = svc.FindUserByID(context.Background(), user.ID)
				Expect(err).ToNot(HaveOccurred())
				Expect(model.IsUserDisabled(updated)).To(BeFalse())
			})
		})

		Context("when email is already in use", func() {
			BeforeEach(func() {
				var user2 model.User
				user, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
				user2, err = svc.CreateUser(context.Background(), params[1])
				Expect(err).ToNot(HaveOccurred())
				update = model.UpdateUserParams{Email: &user2.Email}
			})

			It("returns ErrUserEmailExists error", func() {
				Expect(err).To(MatchError(model.ErrUserEmailExists))
			})
		})

		Context("when user not found", func() {
			It("returns ErrUserNotFound error", func() {
				Expect(err).To(MatchError(model.ErrUserNotFound))
			})
		})
	})

	Describe("user delete", func() {
		var (
			params = testCreateUserParams()
			users  []model.User
			err    error
		)

		JustBeforeEach(func() {
			err = svc.DeleteUserByID(context.Background(), users[0].ID)
		})

		Context("when existing user deleted", func() {
			BeforeEach(func() {
				for _, u := range params {
					user, err := svc.CreateUser(context.Background(), u)
					Expect(err).ToNot(HaveOccurred())
					users = append(users, user)
				}
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("removes user from the database", func() {
				_, err = svc.FindUserByID(context.Background(), users[0].ID)
				Expect(err).To(MatchError(model.ErrUserNotFound))
			})

			It("does not affect other users", func() {
				_, err = svc.FindUserByID(context.Background(), users[1].ID)
				Expect(err).ToNot(HaveOccurred())
			})

			It("allows user with the same email to be created", func() {
				_, err = svc.CreateUser(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when non-existing user deleted", func() {
			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})

func testCreateUserParams() []model.CreateUserParams {
	return []model.CreateUserParams{
		{
			FullName: model.String("John Doe"),
			Email:    "john@example.com",
			Role:     model.ViewerRole,
			Password: "qwerty",
		},
		{
			FullName: model.String("admin"),
			Email:    "admin@local.domain",
			Role:     model.AdminRole,
			Password: "qwerty",
		},
	}
}

func expectUserMatches(user model.User, params model.CreateUserParams) {
	Expect(user.FullName).To(Equal(params.FullName))
	Expect(user.Email).To(Equal(params.Email))
	Expect(user.Role).To(Equal(params.Role))
	Expect(*user.IsDisabled).To(BeFalse())
	Expect(user.CreatedAt).ToNot(BeZero())
	Expect(user.UpdatedAt).ToNot(BeZero())
	Expect(user.LastSeenAt).To(BeZero())
	Expect(user.DeletedAt).To(BeZero())
	Expect(user.PasswordChangedAt).ToNot(BeZero())
	err := model.VerifyPassword(user.PasswordHash, params.Password)
	Expect(err).ToNot(HaveOccurred())
}
