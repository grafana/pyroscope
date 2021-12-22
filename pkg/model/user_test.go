package model_test

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("User password verification", func() {
	Context("when password hashed", func() {
		var (
			p string
			h []byte
		)

		BeforeEach(func() {
			p = "qwerty"
			h = model.MustPasswordHash(p)
		})

		It("produces unique hash", func() {
			Expect(h).ToNot(Equal(model.MustPasswordHash(p)))
		})

		Context("at verification", func() {
			Context("when password matches", func() {
				It("reports no error", func() {
					Expect(model.VerifyPassword(h, p)).ToNot(HaveOccurred())
				})
			})

			Context("when password does not match", func() {
				It("reports error", func() {
					Expect(model.VerifyPassword(h, "xxx")).To(HaveOccurred())
				})
			})
		})
	})
})

var _ = Describe("User validation", func() {
	Describe("CreateUserParams", func() {
		type createUserParamsCase struct {
			params model.CreateUserParams
			err    error
		}

		DescribeTable("CreateUserParams cases",
			func(c createUserParamsCase) {
				expectErrOrNil(c.params.Validate(), c.err)
			},

			Entry("valid params", createUserParamsCase{
				params: model.CreateUserParams{
					Name:     "johndoe",
					FullName: model.String("John Doe"),
					Email:    "john@example.com",
					Password: "qwerty",
				},
			}),

			Entry("name is too long", createUserParamsCase{
				params: model.CreateUserParams{
					Name: strings.Repeat("X", 256),
				},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameTooLong,
					model.ErrUserEmailInvalid,
					model.ErrUserPasswordEmpty,
				}},
			}),

			Entry("full name is too long", createUserParamsCase{
				params: model.CreateUserParams{
					FullName: model.String(strings.Repeat("X", 256)),
				},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameEmpty,
					model.ErrUserEmailInvalid,
					model.ErrUserFullNameTooLong,
					model.ErrUserPasswordEmpty,
				}},
			}),

			Entry("invalid params", createUserParamsCase{
				params: model.CreateUserParams{},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameEmpty,
					model.ErrUserEmailInvalid,
					model.ErrUserPasswordEmpty,
				}},
			}),
		)
	})

	Describe("UpdateUserParams", func() {
		type updateUserParamsCase struct {
			params model.UpdateUserParams
			err    error
		}

		DescribeTable("CreateUserParams cases",
			func(c updateUserParamsCase) {
				expectErrOrNil(c.params.Validate(), c.err)
			},

			Entry("empty params are valid", updateUserParamsCase{
				params: model.UpdateUserParams{},
			}),

			Entry("valid params", updateUserParamsCase{
				params: model.UpdateUserParams{
					Name:     model.String("johndoe"),
					Email:    model.String("john@example.com"),
					FullName: model.String("John Doe"),
					Password: model.String("qwerty")}.
					SetIsDisabled(false),
			}),

			Entry("name is too long", updateUserParamsCase{
				params: model.UpdateUserParams{
					FullName: model.String(strings.Repeat("X", 256)),
				},
				err: model.ErrUserFullNameTooLong,
			}),

			Entry("invalid params", updateUserParamsCase{
				params: model.UpdateUserParams{
					Name:     model.String(""),
					FullName: model.String(""),
					Email:    model.String(""),
					Password: model.String("")},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameEmpty,
					model.ErrUserEmailInvalid,
					model.ErrUserPasswordEmpty,
				}},
			}),
		)
	})
})
