package model_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

var _ = Describe("Role validation", func() {
	type testCase struct {
		role  model.Role
		valid bool
	}

	DescribeTable("Role cases",
		func(c testCase) {
			Expect(c.role.IsValid()).To(Equal(c.valid))
			if c.valid {
				parsed, err := model.ParseRole(c.role.String())
				Expect(err).ToNot(HaveOccurred())
				Expect(parsed).To(Equal(c.role))
			}
		},
		Entry("Invalid", testCase{model.Role(0), false}),
		Entry("Admin", testCase{model.AdminRole, true}),
		Entry("Editor", testCase{model.EditorRole, true}),
		Entry("Viewer", testCase{model.ViewerRole, true}),
	)
})
