package model_test

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
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

var _ = Describe("API key encoding", func() {
	defer GinkgoRecover()

	Describe("psx", func() {
		Context("generated API key can be decoded", func() {
			apiKey := model.APIKey{ID: 13}
			key, hash, err := model.GenerateAPIKey(apiKey.ID)
			Expect(err).ToNot(HaveOccurred())
			apiKey.Hash = hash

			id, secret, err := model.DecodeAPIKey(key)
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(apiKey.ID))
			Expect(apiKey.Verify(secret)).ToNot(HaveOccurred())
		})
	})
})
