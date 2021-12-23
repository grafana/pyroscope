// Package profiler is a public API golang apps should use to send data to pyroscope server. It is intentionally separate from the rest of the code.
//   The idea is that this API won't change much over time, while all the other code will.
package profiler

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
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
	Tags            map[string]string
	ServerAddress   string // e.g http://pyroscope.services.internal:4040
	AuthToken       string // specify this token when using pyroscope cloud
	SampleRate      uint32
	Logger          agent.Logger
	ProfileTypes    []ProfileType
	DisableGCRuns   bool // this will disable automatic runtime.GC runs
}

type Profiler struct {
	session *agent.ProfileSession
}

// Start starts continuously profiling go code
func Start(cfg Config) (*Profiler, error) {
	if len(cfg.ProfileTypes) == 0 {
		cfg.ProfileTypes = types.DefaultProfileTypes
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = types.DefaultSampleRate
	}
	if cfg.Logger == nil {
		cfg.Logger = &agent.NoopLogger{}
	}

	// Override the address to use when the environment variable is defined.
	// This is useful to support adhoc push ingestion.
	if address, ok := os.LookupEnv("PYROSCOPE_ADHOC_SERVER_ADDRESS"); ok {
		cfg.ServerAddress = address
	}

	rc := remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamThreads:        4,
		UpstreamRequestTimeout: 30 * time.Second,
	}
	upstream, err := remote.New(rc, cfg.Logger)
	if err != nil {
		return nil, err
	}

	sc := agent.SessionConfig{
		Upstream:         upstream,
		Logger:           cfg.Logger,
		AppName:          cfg.ApplicationName,
		Tags:             cfg.Tags,
		ProfilingTypes:   cfg.ProfileTypes,
		DisableGCRuns:    cfg.DisableGCRuns,
		SpyName:          types.GoSpy,
		SampleRate:       cfg.SampleRate,
		UploadRate:       10 * time.Second,
		Pid:              0,
		WithSubprocesses: false,
	}
	session, err := agent.NewSession(sc)
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	upstream.Start()
	if err = session.Start(); err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return &Profiler{session: session}, nil
}

// Stop stops continuous profiling session
func (p *Profiler) Stop() error {
	p.session.Stop()
	// FIXME(abeaumont): call upstream.Stop()
	return nil
}

type LabelSet = pprof.LabelSet

var Labels = pprof.Labels

func TagWrapper(ctx context.Context, labels LabelSet, cb func(context.Context)) {
	pprof.Do(ctx, labels, func(c context.Context) { cb(c) })
}
