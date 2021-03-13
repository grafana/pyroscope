package agent

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

const durThreshold = 30 * time.Millisecond

type upstreamMock struct {
	tries []*transporttrie.Trie
}

func (u *upstreamMock) Stop() {

}

func (u *upstreamMock) Upload(name string, startTime, endTime time.Time, spyName string, sampleRate int, t *transporttrie.Trie) {
	u.tries = append(u.tries, t)
}

var _ = Describe("analytics", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("NewSession", func() {
			It("works as expected", func(done Done) {
				u := &upstreamMock{}
				uploadRate := 200 * time.Millisecond
				s := NewSession(u, "test-app", "debugspy", 100, uploadRate, os.Getpid(), true)
				now := time.Now()
				time.Sleep(now.Truncate(uploadRate).Add(uploadRate + 10*time.Millisecond).Sub(now))
				err := s.Start()

				Expect(err).ToNot(HaveOccurred())
				time.Sleep(500 * time.Millisecond)
				s.Stop()

				Expect(u.tries).To(HaveLen(3))
				u.tries[0].Iterate(func(name []byte, val uint64) {
					Expect(val).To(BeNumerically("~", 19, 2))
				})
				u.tries[1].Iterate(func(name []byte, val uint64) {
					Expect(val).To(BeNumerically("~", 20, 2))
				})
				u.tries[2].Iterate(func(name []byte, val uint64) {
					Expect(val).To(BeNumerically("~", 11, 2))
				})
				close(done)
			})
		})
	})
})
