package pyroscope

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/pyroscope-io/client/upstream/remote"
)

type Config struct {
	ApplicationName        string // e.g backend.purchases
	Tags                   map[string]string
	ServerAddress          string // e.g http://pyroscope.services.internal:4040
	AuthToken              string // specify this token when using pyroscope cloud
	BasicAuthUser          string // http basic auth user
	BasicAuthPassword      string // http basic auth password
	TenantID               string // specify TenantId when using phlare multi-tenancy
	SampleRate             uint32 // todo this one is not used
	UploadRate             time.Duration
	Logger                 Logger
	ProfileTypes           []ProfileType
	DisableGCRuns          bool // this will disable automatic runtime.GC runs between getting the heap profiles
	DisableAutomaticResets bool // disable automatic profiler reset every 10 seconds. Reset manually by calling Flush method
	// Deprecated: the field is ignored and does nothing
	DisableCumulativeMerge bool
	HTTPHeaders            map[string]string
}

type Profiler struct {
	session  *Session
	uploader *remote.Remote
}

// Start starts continuously profiling go code
func Start(cfg Config) (*Profiler, error) {
	if len(cfg.ProfileTypes) == 0 {
		cfg.ProfileTypes = DefaultProfileTypes
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = DefaultSampleRate
	}
	if cfg.Logger == nil {
		cfg.Logger = noopLogger
	}

	// Override the address to use when the environment variable is defined.
	// This is useful to support adhoc push ingestion.
	if address, ok := os.LookupEnv("PYROSCOPE_ADHOC_SERVER_ADDRESS"); ok {
		cfg.ServerAddress = address
	}

	rc := remote.Config{
		AuthToken:         cfg.AuthToken,
		TenantID:          cfg.TenantID,
		BasicAuthUser:     cfg.BasicAuthUser,
		BasicAuthPassword: cfg.BasicAuthPassword,
		HTTPHeaders:       cfg.HTTPHeaders,
		Address:           cfg.ServerAddress,
		Threads:           5, // per each profile type upload
		Timeout:           30 * time.Second,
		Logger:            cfg.Logger,
	}
	uploader, err := remote.NewRemote(rc)
	if err != nil {
		return nil, err
	}

	if cfg.UploadRate == 0 {
		// For backward compatibility.
		cfg.UploadRate = 10 * time.Second
	}

	sc := SessionConfig{
		Upstream:               uploader,
		Logger:                 cfg.Logger,
		AppName:                cfg.ApplicationName,
		Tags:                   cfg.Tags,
		ProfilingTypes:         cfg.ProfileTypes,
		DisableGCRuns:          cfg.DisableGCRuns,
		DisableAutomaticResets: cfg.DisableAutomaticResets,
		SampleRate:             cfg.SampleRate,
		UploadRate:             cfg.UploadRate,
	}

	cfg.Logger.Infof("starting profiling session:")
	cfg.Logger.Infof("  AppName:        %+v", sc.AppName)
	cfg.Logger.Infof("  Tags:           %+v", sc.Tags)
	cfg.Logger.Infof("  ProfilingTypes: %+v", sc.ProfilingTypes)
	cfg.Logger.Infof("  DisableGCRuns:  %+v", sc.DisableGCRuns)
	cfg.Logger.Infof("  SampleRate:     %+v", sc.SampleRate)
	cfg.Logger.Infof("  UploadRate:     %+v", sc.UploadRate)
	s, err := NewSession(sc)
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	uploader.Start()
	if err = s.Start(); err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	return &Profiler{session: s, uploader: uploader}, nil
}

// Stop stops continuous profiling session and uploads the remaining profiling data
func (p *Profiler) Stop() error {
	p.session.Stop()
	p.uploader.Stop()
	return nil
}

// Flush resets current profiling session. if wait is true, also waits for all profiles to be uploaded synchronously
func (p *Profiler) Flush(wait bool) {
	p.session.flush(wait)
}

type LabelSet = pprof.LabelSet

var Labels = pprof.Labels

func TagWrapper(ctx context.Context, labels LabelSet, cb func(context.Context)) {
	pprof.Do(ctx, labels, func(c context.Context) { cb(c) })
}
