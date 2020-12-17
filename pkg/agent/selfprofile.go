package agent

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func SelfProfile(cfg *config.Config, u upstream.Upstream, name string) {
	ctrl := NewController(cfg, u)
	ctrl.Start()
	defer ctrl.Stop()
	ctrl.StartContinuousProfiling("gospy", name, 0)
}
