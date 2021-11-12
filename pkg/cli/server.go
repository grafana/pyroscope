package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
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
	healthController *health.Controller
	adminServer      *admin.Server

	stopped chan struct{}
	done    chan struct{}
	group   *errgroup.Group
}

func newServerService(st *storage.Storage, logger *logrus.Logger, c *config.Server, adhoc bool) (*serverService, error) {
	svc := serverService{
		config:  c,
		logger:  logger,
		storage: st,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}

	exportedMetricsRegistry := prometheus.NewRegistry()
	metricsExporter, err := exporter.NewExporter(svc.config.MetricsExportRules, exportedMetricsRegistry)
	if err != nil {
		return nil, fmt.Errorf("new metric exporter: %w", err)
	}

	if adhoc {
		svc.healthController = health.NewController(svc.logger, time.Minute)
	} else {
		if svc.config.EnableExperimentalAdmin {
			socketPath := svc.config.AdminSocketPath
			if socketPath == "" {
				socketPath = filepath.Join(svc.config.StoragePath, "/pyroscope.sock")
			}
			adminSvc := admin.NewService(svc.storage)
			adminCtrl := admin.NewController(svc.logger, adminSvc)
			adminHTTPOverUDS, err := admin.NewUdsHTTPServer(socketPath)
			if err != nil {
				return nil, fmt.Errorf("admin: %w", err)
			}

			svc.adminServer, err = admin.NewServer(
				svc.logger,
				adminCtrl,
				adminHTTPOverUDS,
			)
			if err != nil {
				return nil, fmt.Errorf("admin: %w", err)
			}
		}

		diskPressure := health.DiskPressure{
			Threshold: 512 * bytesize.MB,
			Path:      c.StoragePath,
		}

		svc.healthController = health.NewController(svc.logger, time.Minute, diskPressure)
		svc.debugReporter = debug.NewReporter(svc.logger, svc.storage, prometheus.DefaultRegisterer)
		svc.directUpstream = direct.New(svc.storage, metricsExporter)
		svc.selfProfiling, _ = agent.NewSession(agent.SessionConfig{
			Upstream:       svc.directUpstream,
			AppName:        "pyroscope.server",
			ProfilingTypes: types.DefaultProfileTypes,
			SpyName:        types.GoSpy,
			SampleRate:     100,
			UploadRate:     10 * time.Second,
			Logger:         logger,
		})
	}

	svc.controller, err = server.New(server.Config{
		Configuration:           svc.config,
		Storage:                 svc.storage,
		MetricsExporter:         metricsExporter,
		Notifier:                svc.healthController,
		Logger:                  svc.logger,
		MetricsRegisterer:       prometheus.DefaultRegisterer,
		ExportedMetricsRegistry: exportedMetricsRegistry,
	})
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}

	if !c.AnalyticsOptOut {
		svc.analyticsService = analytics.NewService(c, svc.storage, svc.controller)
	}

	return &svc, nil
}

func (svc *serverService) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	svc.group = g
	g.Go(func() error {
		// if you ever change this line, make sure to update this homebrew test:
		// https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
		svc.logger.Info("starting HTTP server")
		return svc.controller.Start()
	})

	if svc.debugReporter != nil {
		go svc.debugReporter.Start()
	}
	if svc.analyticsService != nil {
		go svc.analyticsService.Start()
	}
	svc.healthController.Start()
	if svc.directUpstream != nil {
		svc.directUpstream.Start()
	}
	if svc.selfProfiling != nil {
		if err := svc.selfProfiling.Start(); err != nil {
			svc.logger.WithError(err).Error("failed to start self-profiling")
		}
	}

	if svc.config.EnableExperimentalAdmin {
		g.Go(func() error {
			svc.logger.Info("starting admin server")
			return svc.adminServer.Start()
		})
	}

	defer close(svc.done)
	select {
	case <-svc.stopped:
	case <-ctx.Done():
		// The context is canceled the first time a function passed to Go
		// returns a non-nil error.
	}
	// N.B. internal components are de-initialized/disposed (if applicable)
	// regardless of the exit reason. Once server is stopped, wait for all
	// Go goroutines to finish.
	svc.stop()
	return svc.group.Wait()
}

func (svc *serverService) Stop() {
	close(svc.stopped)
	<-svc.done
}

//revive:disable-next-line:confusing-naming methods are different
func (svc *serverService) stop() {
	svc.controller.Drain()
	if svc.debugReporter != nil {
		svc.logger.Debug("stopping debug reporter")
		svc.debugReporter.Stop()
	}
	if svc.healthController != nil {
		svc.healthController.Stop()
	}
	if svc.analyticsService != nil {
		svc.logger.Debug("stopping analytics service")
		svc.analyticsService.Stop()
	}
	if svc.selfProfiling != nil {
		svc.logger.Debug("stopping profiling")
		svc.selfProfiling.Stop()
	}
	if svc.directUpstream != nil {
		svc.logger.Debug("stopping upstream")
		svc.directUpstream.Stop()
	}
	svc.logger.Debug("stopping http server")
	if err := svc.controller.Stop(); err != nil {
		svc.logger.WithError(err).Error("controller stop")
	}

	if svc.config.EnableExperimentalAdmin {
		svc.logger.Debug("stopping admin server")
		svc.adminServer.Stop()
	}
}
