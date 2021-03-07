package agent

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
)

func SelfProfile(_ *config.Config, u upstream.Upstream, appName string, logger Logger) error {
	// TODO: add sample rate
	s := NewSession(u, appName, "gospy", 100, 0, false)
	err := s.Start()

	s.Logger = logger

	if err != nil {
		return err
	}

	atexit.Register(s.Stop)
	return nil
}
