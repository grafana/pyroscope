// Package profiler is a public API golang apps should use to send data to pyroscope server. It is intentionally separate from the rest of the code.
//   The idea is that this API won't change much over time, while all the other code will.
package profiler

import (
	"context"
	"fmt"
	"runtime/pprof"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	pyprof "github.com/pyroscope-io/pyroscope/pkg/agent/pprof"
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
	if err = session.Start(); err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return &Profiler{session: session}, nil
}

// Stop stops continuous profiling session
func (p *Profiler) Stop() error {
	p.session.Stop()
	return nil
}

type Labels map[string]string

// SetLabels overrides goroutine labels.
func SetLabels(labels Labels) { pyprof.SetGoroutineLabels(labels) }

// Context returns a new context.Context with the goroutine labels added.
// The context is intended for interoperability with pprof.Do call.
func Context(ctx context.Context) context.Context {
	gl := pyprof.GetGoroutineLabels()
	pl := make([]string, 0, len(gl)*2)
	for k, v := range gl {
		pl = append(pl, k)
		pl = append(pl, v)
	}
	return pprof.WithLabels(ctx, pprof.Labels(pl...))
}

// WithLabels calls f with the given labels added to the parent's label map.
//
// Goroutines spawned while executing f will inherit the augmented label-set.
// Each key/value pair in labels is inserted into the label map in the order
// provided, overriding any previous value for the same key.
//
// The augmented label map will be set for the duration of the call to f
// and restored once f returns.
func WithLabels(labels Labels, f func()) {
	m := pyprof.GetGoroutineLabels()
	defer pyprof.SetGoroutineLabels(m)
	c := make(Labels)
	for k, v := range m {
		c[k] = v
	}
	for k, v := range labels {
		c[k] = v
	}
	pyprof.SetGoroutineLabels(c)
	f()
}
