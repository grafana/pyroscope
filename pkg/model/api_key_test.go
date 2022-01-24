package model_test

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"

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
