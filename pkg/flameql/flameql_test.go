package flameql

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TagMatcher", func() {
	It("matches", func() {
		type testCase struct {
			matcher string
			value   string
			matches bool
		}

		testCases := []testCase{
			{`foo="bar"`, "bar", true},
			{`foo="bar"`, "baz", false},
			{`foo!="bar"`, "bar", false},
			{`foo!="bar"`, "baz", true},
			{`foo=~"bar"`, "bar", true},
			{`foo=~"bar"`, "baz", false},
			{`foo!~"bar"`, "bar", false},
			{`foo!~"bar"`, "baz", true},
		}

		for _, tc := range testCases {
			m, err := ParseMatchers(tc.matcher)
			Expect(err).ToNot(HaveOccurred())
			Expect(m[0].Match(tc.value)).To(Equal(tc.matches))
		}
	})
})
