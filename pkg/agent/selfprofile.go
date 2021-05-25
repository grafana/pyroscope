package agent

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
)

func SelfProfile(sampleRate uint32, u upstream.Upstream, appName string, logger Logger) error {
	// TODO: upload rate should come from config
	c := SessionConfig{
		Upstream:         u,
		AppName:          appName,
		ProfilingTypes:   types.DefaultProfileTypes,
		SpyName:          types.GoSpy,
		SampleRate:       sampleRate,
		UploadRate:       10 * time.Second,
		Pid:              0,
		WithSubprocesses: false,
	}
	s := NewSession(&c)
	if err := s.Start(); err != nil {
		return err
	}
	s.Logger = logger

	atexit.Register(s.Stop)
	return nil
}
