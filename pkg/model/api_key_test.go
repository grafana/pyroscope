package model_test

import (
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("API key validation", func() {
	type testCase struct {
		params model.CreateAPIKeyParams
		err    error
	}

	validateParams := func(c testCase) {
		expectErrOrNil(c.params.Validate(), c.err)
	}

	DescribeTable("API key cases",
		validateParams,

		Entry("valid params", testCase{
			params: model.CreateAPIKeyParams{
				Name: "johndoe",
				Role: model.ReadOnlyRole,
			},
		}),

		Entry("name is too long", testCase{
			params: model.CreateAPIKeyParams{
				Name: strings.Repeat("X", 256),
			},
			err: &multierror.Error{Errors: []error{
				model.ErrAPIKeyNameTooLong,
				model.ErrRoleUnknown,
			}},
		}),

		Entry("invalid params", testCase{
			params: model.CreateAPIKeyParams{},
			err: &multierror.Error{Errors: []error{
				model.ErrAPIKeyNameEmpty,
				model.ErrRoleUnknown,
			}},
		}),
	)
})

var _ = Describe("API key JWT encoding", func() {
	var p model.CreateAPIKeyParams
	BeforeEach(func() {
		p = model.CreateAPIKeyParams{
			Name: "api_key_name",
			Role: model.AdminRole,
		}
	})

	Context("when a new token is generated for an API key", func() {
		It("produces a valid JWT token", func() {
			t := p.JWTToken()
			s := []byte("signing-key")
			signed, signature, err := model.SignJWTToken(t, s)
			Expect(err).ToNot(HaveOccurred())

			parsed, parseErr := jwt.Parse(signed, func(token *jwt.Token) (interface{}, error) {
				Expect(token.Method.Alg()).To(Equal(jwt.SigningMethodHS256.Alg()))
				return s, nil
			})

			Expect(parseErr).ToNot(HaveOccurred())
			Expect(parsed.Signature).To(Equal(signature))
			Expect(parsed.Valid).To(BeTrue())
		})
	})

	Context("when a token is parsed with APIKeyFromJWTToken", func() {
		It("returns false if API key can not be retrieved", func() {
			p = model.CreateAPIKeyParams{}
			_, ok := model.APIKeyFromJWTToken(p.JWTToken())
			Expect(ok).To(BeFalse())
		})

		It("creates a valid API key if the token is valid", func() {
			apiKey, ok := model.APIKeyFromJWTToken(p.JWTToken())
			Expect(ok).To(BeTrue())
			Expect(apiKey).To(Equal(model.TokenAPIKey{
				Name: p.Name,
				Role: p.Role,
			}))
		})
	})
})
