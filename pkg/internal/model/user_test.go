package model_test

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
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
					FullName: model.String("John Doe"),
					Email:    "john@example.com",
					Password: "qwerty",
					Role:     model.ViewerRole,
				},
			}),
			Entry("name is too long", createUserParamsCase{
				params: model.CreateUserParams{
					FullName: model.String(strings.Repeat("X", 256)),
				},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameTooLong,
					model.ErrUserPasswordEmpty,
					model.ErrUserEmailInvalid,
					model.ErrRoleUnknown,
				}},
			}),
			Entry("invalid params", createUserParamsCase{
				params: model.CreateUserParams{
					FullName: model.String(""),
				},
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameEmpty,
					model.ErrUserPasswordEmpty,
					model.ErrUserEmailInvalid,
					model.ErrRoleUnknown,
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
					FullName: model.String("John Doe"),
					Email:    model.String("john@example.com"),
					Password: model.String("qwerty")}.
					SetIsDisabled(false).
					SetRole(model.ViewerRole),
			}),
			Entry("name is too long", updateUserParamsCase{
				params: model.UpdateUserParams{
					FullName: model.String(strings.Repeat("X", 256)),
				},
				err: model.ErrUserNameTooLong,
			}),
			Entry("invalid params", updateUserParamsCase{
				params: model.UpdateUserParams{
					FullName: model.String(""),
					Email:    model.String(""),
					Password: model.String("")}.
					SetRole(model.Role(0)),
				err: &multierror.Error{Errors: []error{
					model.ErrUserNameEmpty,
					model.ErrUserEmailInvalid,
					model.ErrUserPasswordEmpty,
					model.ErrRoleUnknown,
				}},
			}),
		)
	})
})
