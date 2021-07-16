package flameql

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateKey", func() {
	It("reports error if a key violates constraints", func() {
		type testCase struct {
			key string
			err error
		}

		testCases := []testCase{
			{"foo/BAR.1-2.baz_qux", nil},

			{ReservedKeyName, ErrKeyReserved},
			{"", ErrKeyIsRequired},
			{"#", ErrInvalidKey},
		}

		for _, tc := range testCases {
			err := ValidateKey(tc.key)
			if tc.err != nil {
				Expect(errors.Is(err, tc.err)).To(BeTrue())
				continue
			}
			Expect(err).To(BeNil())
		}
	})
})
