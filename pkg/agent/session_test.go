package agent

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
	"github.com/sirupsen/logrus"
)

const durThreshold = 30 * time.Millisecond

type upstreamMock struct {
	tries []*transporttrie.Trie
}

func (*upstreamMock) Stop() {

}

func (u *upstreamMock) Upload(j *upstream.UploadJob) {
	u.tries = append(u.tries, j.Trie)
}

var _ = Describe("agent.Session", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("NewSession", func() {
			It("creates a new session and performs chunking", func(done Done) {
				u := &upstreamMock{}
				uploadRate := 200 * time.Millisecond
				s := NewSession(&SessionConfig{
					Upstream:         u,
					AppName:          "test-app",
					ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
					SpyName:          "debugspy",
					SampleRate:       100,
					UploadRate:       uploadRate,
					Pid:              os.Getpid(),
					WithSubprocesses: true,
				}, logrus.StandardLogger())
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
