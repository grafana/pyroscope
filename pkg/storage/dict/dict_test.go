package dict

import (
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TODO: DRY
func randStr() []byte {
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
	return buf
}

var _ = Describe("dict package", func() {
	Context("Put / Get", func() {
		Context("Puts same value twice", func() {
			It("Get returns things Put puts in", func() {
				dict := New()
				k1 := dict.Store([]byte("foo"))
				k2 := dict.Store([]byte("aff"))
				k3 := dict.Store([]byte("aff"))
				v1, _ := dict.Load(k1)
				Expect(v1).To(BeEquivalentTo([]byte("foo")))
				v2, _ := dict.Load(k2)
				Expect(v2).To(BeEquivalentTo([]byte("aff")))
				v3, _ := dict.Load(k3)
				Expect(v3).To(BeEquivalentTo([]byte("aff")))
			})
		})

		type testCase struct {
			key      uint64
			expected []byte
		}
		Context("Random strings", func() {
			It("Get returns things Put puts in", func() {
				dict := New()
				var insertedData []testCase
				for i := 0; i < 10000; i++ {
					expected := randStr()
					key := dict.Store(expected)
					insertedData = append(insertedData, testCase{key, expected})
					actual, ok := dict.Load(key)
					Expect(ok).To(BeTrue())
					Expect(actual).To(BeEquivalentTo(expected))
				}

				for _, tc := range insertedData {
					actual, ok := dict.Load(tc.key)
					Expect(ok).To(BeTrue())
					Expect(actual).To(BeEquivalentTo(tc.expected))
				}
			})
		})
	})
})
