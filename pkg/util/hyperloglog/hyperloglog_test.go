package hyperloglog_test

import (
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	wrapper "github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
	"github.com/spaolacci/murmur3"
)

type hashString string

func (hs hashString) Sum64() uint64 {
	return murmur3.Sum64WithSeed([]byte(hs), 123)
}

var _ = Describe("Hyperloglog", func() {
	defer GinkgoRecover()
	Context("wrapper implementation", func() {
		It("panics", func() {
			h, _ := wrapper.NewPlus(18)
			panics := 0
			count := 100
			wg := sync.WaitGroup{}
			wg.Add(count)
			for i := 0; i < count; i++ {
				go func() {
					defer wg.Done()
					h.Add(hashString("test"))
				}()
			}
			wg.Wait()
			Expect(panics).To(Equal(0))
		})
	})
})
