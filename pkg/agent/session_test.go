package agent

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/sirupsen/logrus"
)

type upstreamMock struct {
	uploads []*upstream.UploadJob
}

func (*upstreamMock) Stop() {}

func (u *upstreamMock) Upload(j *upstream.UploadJob) {
	u.uploads = append(u.uploads, j)
}

var _ = Describe("agent.Session", func() {
	Describe("NewSession", func() {
		It("creates a new session and performs chunking", func(done Done) {
			u := &upstreamMock{}
			uploadRate := 200 * time.Millisecond
			s, _ := NewSession(&SessionConfig{
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

			Expect(u.uploads).To(HaveLen(3))
			u.uploads[0].Trie.Iterate(func(name []byte, val uint64) {
				Expect(val).To(BeNumerically("~", 19, 2))
			})
			u.uploads[1].Trie.Iterate(func(name []byte, val uint64) {
				Expect(val).To(BeNumerically("~", 20, 2))
			})
			u.uploads[2].Trie.Iterate(func(name []byte, val uint64) {
				Expect(val).To(BeNumerically("~", 11, 2))
			})
			close(done)
		})

		When("tags specified", func() {
			It("name ", func() {
				u := &upstreamMock{}
				uploadRate := 200 * time.Millisecond
				c := &SessionConfig{
					Upstream:         u,
					AppName:          "test-app{bar=xxx}",
					ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
					SpyName:          "debugspy",
					SampleRate:       100,
					UploadRate:       uploadRate,
					Pid:              os.Getpid(),
					WithSubprocesses: true,
					Tags: map[string]string{
						"foo": "bar",
						"baz": "qux",
					},
				}

				s, _ := NewSession(c, logrus.StandardLogger())
				now := time.Now()
				time.Sleep(now.Truncate(uploadRate).Add(uploadRate + 10*time.Millisecond).Sub(now))
				err := s.Start()
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(500 * time.Millisecond)
				s.Stop()

				Expect(u.uploads).To(HaveLen(3))
				Expect(u.uploads[0].Name).To(Equal("test-app.cpu{bar=xxx,baz=qux,foo=bar}"))
			})
		})
	})
})
