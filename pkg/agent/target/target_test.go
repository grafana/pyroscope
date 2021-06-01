// +build debugspy

package target

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type fakeTarget struct{ attached int }

func (t *fakeTarget) attach(_ context.Context) { t.attached++ }

var _ = Describe("target", func() {
	It("Attaches to targets", func() {
		tgtMgr := NewManager(logrus.StandardLogger(), new(remote.Remote), &config.Agent{
			Targets: []config.Target{
				{
					ServiceName:     "my-service",
					SpyName:         "debugspy",
					ApplicationName: "my.app",
				},
			},
		})

		t := new(fakeTarget)
		tgtMgr.resolve = func(c config.Target) (target, bool) { return t, true }
		tgtMgr.backoffPeriod = time.Millisecond * 10

		tgtMgr.Start()
		time.Sleep(time.Second)
		tgtMgr.Stop()

		Expect(t.attached).ToNot(BeZero())
	})
})
