package flameql

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateTagKey", func() {
	It("reports error if a key violates constraints", func() {
		type testCase struct {
			key string
			err error
		}

		testCases := []testCase{
			{"foo/BAR.1-2.baz_qux", nil},

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
