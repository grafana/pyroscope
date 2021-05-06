package hyperloglog_test

import (
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	wrapper "github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
	"github.com/twmb/murmur3"
)

type hashString string

func (hs hashString) Sum64() uint64 {
	return murmur3.SeedSum64(123, []byte(hs))
}

var _ = Describe("Hyperloglog", func() {
	Context("wrapper implementation", func() {
		// original implementation panics with "concurrent map writes"
		It("doesn't panic", func(done Done) {
			Expect(func() {
				h, _ := wrapper.NewPlus(18)
				count := 10000
				wg := sync.WaitGroup{}
				wg.Add(count)
				for i := 0; i < count; i++ {
					go func() {
						h.Add(hashString("test"))
						wg.Done()
					}()
				}
				wg.Wait()
			}).ToNot(Panic())
			close(done)
		})
	})
})
