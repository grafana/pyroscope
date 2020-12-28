// Package profiler is a public API golang apps should use to send data to pyroscope server. It is intentionally separate from the rest of the code.
//   The idea is that this API won't change much over time, while all the other code will.
package profiler

import (
	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type Config struct {
	ServiceName   string // e.g backend.purchases
	ServerAddress string // e.g http://pyroscope.services.internal:8080
}

type Profiler struct {
	ctrl *agent.Controller
}

// Start starts continuously profiling go code
func Start(cfg Config) (*Profiler, error) {
	globalConfig := &config.Config{
		Agent: config.Agent{
			UpstreamAddress: cfg.ServerAddress,
			UpstreamThreads: 4,
		},
	}
	u := remote.New(globalConfig)
	ctrl := agent.NewController(globalConfig, u)
	ctrl.Start()
	go ctrl.StartContinuousProfiling("gospy", cfg.ServiceName, 0, false)

	p := &Profiler{
		ctrl: ctrl,
	}

	return p, nil
}

// Stop stops continious profiling session
func (p *Profiler) Stop() error {
	p.ctrl.Stop()
	return nil
}
