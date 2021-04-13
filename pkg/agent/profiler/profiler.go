// Package profiler is a public API golang apps should use to send data to pyroscope server. It is intentionally separate from the rest of the code.
//   The idea is that this API won't change much over time, while all the other code will.
package profiler

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
)

type Config struct {
	ApplicationName string // e.g backend.purchases
	ServerAddress   string // e.g http://pyroscope.services.internal:4040
	AuthToken       string
	Logger          agent.Logger
}

type Profiler struct {
	sess *agent.ProfileSession
}

// Start starts continuously profiling go code
func Start(cfg Config) (*Profiler, error) {
	u, err := remote.New(remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamThreads:        4,
		UpstreamRequestTimeout: 30 * time.Second,
	})

	u.Logger = cfg.Logger

	if err != nil {
		return nil, err
	}

	// TODO: add sample rate
	c := agent.SessionConfig{
		Upstream:         u,
		AppName:          cfg.ApplicationName,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU, spy.ProfileAllocObjects, spy.ProfileAllocSpace, spy.ProfileInuseObjects, spy.ProfileInuseSpace},
		SpyName:          "gospy",
		SampleRate:       100,
		UploadRate:       10 * time.Second,
		Pid:              0,
		WithSubprocesses: false,
	}
	sess := agent.NewSession(&c)
	sess.Logger = cfg.Logger
	sess.Start()

	p := &Profiler{
		sess: sess,
	}

	return p, nil
}

// Stop stops continious profiling session
func (p *Profiler) Stop() error {
	p.sess.Stop()
	return nil
}
