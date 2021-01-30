package cappedarr

import (
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("cappedarr", func() {
	defer GinkgoRecover()
	Context("simple case", func() {
		It("works", func() {
			values := []uint64{1, 2, 3, 4, 5, 6}
			for i := 0; i < 1000; i++ {
				ca := New(4)
				rand.Seed(time.Now().UnixNano())
				rand.Shuffle(len(values), func(i, j int) {
					values[i], values[j] = values[j], values[i]
				})

				for _, v := range values {
					ca.Push(v)
				}

				Expect(ca.MinValue()).To(Equal(uint64(3)))
			}
		})
	})
})
