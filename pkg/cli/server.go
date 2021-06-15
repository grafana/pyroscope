package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/debug"
)

type serverService struct {
	config           *config.Server
	logger           *logrus.Logger
	controller       *server.Controller
	storage          *storage.Storage
	directUpstream   *direct.Direct
	analyticsService *analytics.Service
	selfProfiling    *agent.ProfileSession
	debugReporter    *debug.Reporter

	stopped chan struct{}
	done    chan struct{}
	group   *errgroup.Group
}

func newServerService(logger *logrus.Logger, c *config.Server) (*serverService, error) {
	svc := serverService{
		config:  c,
		logger:  logger,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}

	var err error
	// TODO: start and stop storage maintenance (GC/cache/retention) separately.
	svc.storage, err = storage.New(svc.config)
	if err != nil {
		return nil, fmt.Errorf("new storage: %v", err)
	}
	svc.controller, err = server.New(svc.config, svc.storage)
	if err != nil {
		return nil, fmt.Errorf("new server: %v", err)
	}

	svc.debugReporter = debug.NewReporter(svc.logger, svc.storage, svc.config)
	svc.directUpstream = direct.New(svc.storage)
	selfProfilingConfig := &agent.SessionConfig{
		Upstream:       svc.directUpstream,
		AppName:        "pyroscope.server",
		ProfilingTypes: types.DefaultProfileTypes,
		SpyName:        types.GoSpy,
		SampleRate:     uint32(c.SampleRate),
		UploadRate:     10 * time.Second,
	}
	svc.selfProfiling = agent.NewSession(selfProfilingConfig, svc.logger)
	if !c.AnalyticsOptOut {
		svc.analyticsService = analytics.NewService(c, svc.storage, svc.controller)
	}

	return &svc, nil
}

func (svc *serverService) Start() error {
	g, ctx := errgroup.WithContext(context.Background())
	svc.group = g

	g.Go(func() error {
		// if you ever change this line, make sure to update this homebrew test:
		// https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
		logrus.Info("starting HTTP server")
		return svc.controller.Start()
	})

	g.Go(func() error {
		svc.debugReporter.Start()
		return nil
	})

	g.Go(func() error {
		svc.analyticsService.Start()
		return nil
	})

	// Self-profiling runs concurrently, its failure does not cause server to stop.
	svc.directUpstream.Start()
	if err := svc.selfProfiling.Start(); err != nil {
		svc.logger.WithError(err).Error("failed to start self-profiling")
	}

	defer close(svc.done)
	select {
	case <-svc.stopped:
	case <-ctx.Done():
		// The context is canceled the first time a function passed to Go
		// returns a non-nil error or the first time Wait returns, whichever
		// occurs first.
	}
	svc.stop()
	return svc.group.Wait()
}

func (svc *serverService) Stop() {
	close(svc.stopped)
	<-svc.done
}

func (svc *serverService) stop() {
	if svc.debugReporter != nil {
		svc.debugReporter.Stop()
	}
	if svc.analyticsService != nil {
		svc.analyticsService.Stop()
	}
	if svc.selfProfiling != nil {
		svc.selfProfiling.Stop()
	}
	if svc.directUpstream != nil {
		svc.directUpstream.Stop()
	}
	if svc.controller != nil {
		if err := svc.controller.Stop(); err != nil {
			svc.logger.WithError(err).Error("controller stop")
		}
	}
	if svc.storage != nil {
		if err := svc.storage.Close(); err != nil {
			svc.logger.WithError(err).Error("storage close")
		}
	}
}
