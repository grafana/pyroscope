// Package profiler is a public API golang apps should use to send data to pyroscope server. It is intentionally separate from the rest of the code.
//   The idea is that this API won't change much over time, while all the other code will.
package profiler

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
)

type ProfileType = spy.ProfileType

var (
	ProfileCPU          = spy.ProfileCPU
	ProfileAllocObjects = spy.ProfileAllocObjects
	ProfileAllocSpace   = spy.ProfileAllocSpace
	ProfileInuseObjects = spy.ProfileInuseObjects
	ProfileInuseSpace   = spy.ProfileInuseSpace
)

type Config struct {
	ApplicationName string // e.g backend.purchases
	ServerAddress   string // e.g http://pyroscope.services.internal:4040
	AuthToken       string // specify this token when using pyroscope cloud
	SampleRate      uint32
	Logger          agent.Logger
	ProfileTypes    []ProfileType
	DisableGCRuns   bool // this will disable automatic runtime.GC runs
}

type Profiler struct {
	sess *agent.ProfileSession
}

// Start starts continuously profiling go code
func Start(cfg Config) (*Profiler, error) {
	if len(cfg.ProfileTypes) == 0 {
		cfg.ProfileTypes = types.DefaultProfileTypes
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = types.DefaultSampleRate
	}

	u, err := remote.New(remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamThreads:        4,
		UpstreamRequestTimeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	u.Logger = cfg.Logger

	c := agent.SessionConfig{
		Upstream:         u,
		AppName:          cfg.ApplicationName,
		ProfilingTypes:   types.DefaultProfileTypes,
		DisableGCRuns:    cfg.DisableGCRuns,
		SpyName:          types.GoSpy,
		SampleRate:       cfg.SampleRate,
		UploadRate:       10 * time.Second,
		Pid:              0,
		WithSubprocesses: false,
	}
	sess := agent.NewSession(&c)

	sess.Logger = cfg.Logger
	if err := sess.Start(); err != nil {
		return nil, err
	}

	return &Profiler{
		sess: sess,
	}, nil
}

// Stop stops continious profiling session
func (p *Profiler) Stop() error {
	p.sess.Stop()
	return nil
}
