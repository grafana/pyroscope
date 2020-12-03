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
				k1 := dict.Put([]byte("foo"))
				k2 := dict.Put([]byte("aff"))
				k3 := dict.Put([]byte("aff"))
				v1, _ := dict.Get(k1)
				Expect(v1).To(BeEquivalentTo([]byte("foo")))
				v2, _ := dict.Get(k2)
				Expect(v2).To(BeEquivalentTo([]byte("aff")))
				v3, _ := dict.Get(k3)
				Expect(v3).To(BeEquivalentTo([]byte("aff")))
			})
		})

		Context("Random strings", func() {
			It("Get returns things Put puts in", func() {
				dict := New()
				insertedData := [][][]byte{}
				for i := 0; i < 10000; i++ {
					expected := randStr()
					key := dict.Put(expected)
					insertedData = append(insertedData, [][]byte{key, expected})
					actual, ok := dict.Get(key)
					Expect(ok).To(BeTrue())
					Expect(actual).To(BeEquivalentTo(expected))
				}

				for _, pair := range insertedData {
					key := pair[0]
					expected := pair[1]
					actual, ok := dict.Get(key)
					Expect(ok).To(BeTrue())
					Expect(actual).To(BeEquivalentTo(expected))
				}
			})
		})
	})
})
