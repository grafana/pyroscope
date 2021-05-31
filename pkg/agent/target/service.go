package target

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

type service struct {
	logger agent.Logger
	t      *config.Target
	c      *config.Exec
}

func newServiceTarget(logger agent.Logger, c *config.Agent, t config.Target) *service {
	return &service{
		logger: logger,
		c: &config.Exec{
			SpyName:                t.SpyName,
			ApplicationName:        t.ApplicationName,
			SampleRate:             t.SampleRate,
			DetectSubprocesses:     t.DetectSubprocesses,
			PyspyBlocking:          t.PyspyBlocking,
			LogLevel:               c.LogLevel,
			ServerAddress:          c.ServerAddress,
			AuthToken:              c.AuthToken,
			UpstreamThreads:        c.UpstreamThreads,
			UpstreamRequestTimeout: c.UpstreamRequestTimeout,
			NoLogging:              c.NoLogging,
		},
	}
}

func (s *service) attach(ctx context.Context) {
	pid, err := getPID(s.t.ServiceName)
	if err == nil {
		s.logger.Debugf("found service %q process %d", s.t.ServiceName, pid)
		s.c.Pid = pid
		err = exec.Cli(ctx, s.c, nil)
	}
	if err != nil {
		s.logger.Errorf("failed to attach to service %q: %v", s.t.ServiceName, err)
	}
}
