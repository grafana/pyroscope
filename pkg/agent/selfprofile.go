package agent

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
)

func SelfProfile(_ *config.Config, u upstream.Upstream, appName string, logger Logger) error {
	// TODO: sample rate and upload rate should come from config
	c := SessionConfig{
		Upstream:         u,
		AppName:          appName,
		ProfilingTypes:   []string{"cpu", "inuse_objects", "alloc_objects", "inuse_space", "alloc_space"},
		SpyName:          "gospy",
		SampleRate:       100,
		UploadRate:       10 * time.Second,
		Pid:              0,
		WithSubprocesses: false,
	}
	s := NewSession(&c)
	err := s.Start()

	s.Logger = logger

	if err != nil {
		return err
	}

	atexit.Register(s.Stop)
	return nil
}
