package agent

import (
	"time"

	"github.com/petethepig/pyroscope/pkg/agent/upstream"
	"github.com/petethepig/pyroscope/pkg/config"
)

func SelfProfile(cfg *config.Config, u upstream.Upstream, name string) {
	ctrl := newController(cfg, u)
	ctrl.Start()
	for {
		profileID := ctrl.StartProfiling("gospy", 0)
		time.Sleep(10 * time.Second)
		ctrl.StopProfiling(profileID, name)
	}
}
