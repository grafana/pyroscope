package flameql

import (
	"errors"

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

var _ = Describe("ValidateTagKey", func() {
	It("reports error if a key violates constraints", func() {
		type testCase struct {
			key string
			err error
		}

		testCases := []testCase{
			{"foo_BAR_12_baz_qux", nil},

			{ReservedTagKeyName, ErrTagKeyReserved},
			{"", ErrTagKeyIsRequired},
			{"#", ErrInvalidTagKey},
		}

		for _, tc := range testCases {
			err := ValidateTagKey(tc.key)
			if tc.err != nil {
				Expect(errors.Is(err, tc.err)).To(BeTrue())
				continue
			}
			Expect(err).To(BeNil())
		}
	})
})

var _ = Describe("ValidateAppName", func() {
	It("reports error if an app name violates constraints", func() {
		type testCase struct {
			appName string
			err     error
		}

		testCases := []testCase{
			{"foo.BAR-1.2_baz_qux", nil},

			{"", ErrAppNameIsRequired},
			{"#", ErrInvalidAppName},
		}

		for _, tc := range testCases {
			err := ValidateAppName(tc.appName)
			if tc.err != nil {
				Expect(errors.Is(err, tc.err)).To(BeTrue())
				continue
			}
			Expect(err).To(BeNil())
		}
	})
})
