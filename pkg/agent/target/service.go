package target

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mitchellh/go-ps"
	"github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var (
	ErrNotFound   = errors.New("service is not found")
	ErrNotRunning = errors.New("service is not running")
)

type service struct {
	logger *logrus.Logger
	target config.Target
	sc     *agent.SessionConfig
}

func newServiceTarget(logger *logrus.Logger, upstream *remote.Remote, t config.Target) *service {
	return &service{
		logger: logger,
		target: t,
		sc: &agent.SessionConfig{
			Upstream:         upstream,
			AppName:          t.ApplicationName,
			ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
			SpyName:          t.SpyName,
			SampleRate:       uint32(t.SampleRate),
			UploadRate:       10 * time.Second,
			WithSubprocesses: t.DetectSubprocesses,
			// PID to be specified.
		},
	}
}

func (s *service) attach(ctx context.Context) {
	logger := s.logger.WithFields(logrus.Fields{
		"service-name": s.target.ServiceName,
		"app-name":     s.sc.AppName,
		"spy-name":     s.sc.SpyName})
	pid, err := getPID(s.target.ServiceName)
	if err == nil {
		logger.WithField("pid", pid).Debug("starting session")
		s.sc.Pid = pid
		err = s.wait(ctx)
	}
	if err != nil {
		logger.WithError(err).Error("failed to attach spy to service")
	} else {
		logger.Debug("session ended")
	}
}

func (s *service) wait(ctx context.Context) error {
	// TODO: this is somewhat hacky, we need to find a better way to configure agents
	pyspy.Blocking = s.target.PyspyBlocking
	rbspy.Blocking = s.target.RbspyBlocking

	session := agent.NewSession(s.sc, s.logger)
	if err := session.Start(); err != nil {
		return err
	}
	defer session.Stop()

	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p, err := ps.FindProcess(s.sc.Pid)
			if err != nil {
				return fmt.Errorf("could not find process: %w", err)
			}
			if p == nil && err == nil {
				return nil
			}
		}
	}
}
