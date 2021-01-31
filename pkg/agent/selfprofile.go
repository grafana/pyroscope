package agent

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
	"github.com/sirupsen/logrus"
)

func SelfProfile(_ *config.Config, u upstream.Upstream, appName string) {
	// TODO: add sample rate
	s := NewSession(u, appName, "gospy", 100, 0, false)
	err := s.Start()

	if err != nil {
		logrus.Errorf("failed to start profiling session: %s", err)
		return
	}

	atexit.Register(s.Stop)
}
