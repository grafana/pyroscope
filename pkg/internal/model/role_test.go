package model_test

import (
	"encoding/json"

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

	validateRole := func(c testCase) {
		Expect(c.role.IsValid()).To(Equal(c.valid))
		parsed, err := model.ParseRole(c.role.String())
		Expect(parsed).To(Equal(c.role))
		if c.valid {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(MatchError(model.ErrRoleUnknown))
		}
	}

	DescribeTable("Role cases",
		validateRole,
		Entry("Invalid", testCase{model.InvalidRole, false}),
		Entry("Admin", testCase{model.AdminRole, true}),
		Entry("Editor", testCase{model.EditorRole, true}),
		Entry("Viewer", testCase{model.ViewerRole, true}),
	)
})

var _ = Describe("Role JSON", func() {
	type testCase struct {
		json string
		role model.Role
	}

	type role struct {
		Role model.Role
	}

	Context("when a role is marshaled with JSON encoder", func() {
		expectEncoded := func(c testCase) {
			b, err := json.Marshal(role{c.role})
			Expect(err).ToNot(HaveOccurred())
			Expect(b).To(MatchJSON(c.json))
		}

		DescribeTable("JSON marshal cases",
			expectEncoded,
			Entry("Valid", testCase{`{"Role":"Admin"}`, model.AdminRole}),
			Entry("Invalid", testCase{`{"Role":""}`, model.InvalidRole}),
		)
	})

	Context("when a JSON encoded role is unmarshalled", func() {
		expectDecoded := func(c testCase) {
			var x role
			err := json.Unmarshal([]byte(c.json), &x)
			Expect(err).ToNot(HaveOccurred())
			Expect(x.Role).To(Equal(c.role))
		}

		DescribeTable("JSON unmarshal cases",
			expectDecoded,
			Entry("Valid", testCase{`{"Role":"Admin"}`, model.AdminRole}),
			Entry("Invalid", testCase{`{"Role":"NotAValidRole"}`, model.InvalidRole}),
		)
	})
})
