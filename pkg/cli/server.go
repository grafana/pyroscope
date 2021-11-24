package cli

import (
	"context"
	"fmt"
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

func newServerService(logger *logrus.Logger, c *config.Server) (*serverService, error) {
	svc := serverService{
		config:  c,
		logger:  logger,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}

	var err error
	svc.storage, err = storage.New(storage.NewConfig(svc.config), svc.logger, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, fmt.Errorf("new storage: %w", err)
	}

	// this needs to happen after storage is initiated!
	if svc.config.EnableExperimentalAdmin {
		socketPath := svc.config.AdminSocketPath
		adminSvc := admin.NewService(svc.storage)
		adminCtrl := admin.NewController(svc.logger, adminSvc)
		httpClient, err := admin.NewHTTPOverUDSClient(socketPath)
		if err != nil {
			return nil, fmt.Errorf("admin: %w", err)
		}

		adminHTTPOverUDS, err := admin.NewUdsHTTPServer(socketPath, httpClient)
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

	exportedMetricsRegistry := prometheus.NewRegistry()
	metricsExporter, err := exporter.NewExporter(svc.config.MetricsExportRules, exportedMetricsRegistry)
	if err != nil {
		return nil, fmt.Errorf("new metric exporter: %w", err)
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

func (svc *serverService) Start() error {
	g, ctx := errgroup.WithContext(context.Background())
	svc.group = g
	g.Go(func() error {
		// if you ever change this line, make sure to update this homebrew test:
		// https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
		svc.logger.Info("starting HTTP server")
		return svc.controller.Start()
	})

	go svc.debugReporter.Start()
	if svc.analyticsService != nil {
		go svc.analyticsService.Start()
	}

	svc.healthController.Start()
	svc.directUpstream.Start()
	if err := svc.selfProfiling.Start(); err != nil {
		svc.logger.WithError(err).Error("failed to start self-profiling")
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
	if svc.config.EnableExperimentalAdmin {
		svc.logger.Debug("stopping admin server")
		svc.adminServer.Stop()
	}

	svc.controller.Drain()
	svc.logger.Debug("stopping http server")
	if err := svc.controller.Stop(); err != nil {
		svc.logger.WithError(err).Error("controller stop")
	}
	svc.logger.Debug("stopping debug reporter")
	svc.debugReporter.Stop()
	svc.healthController.Stop()
	if svc.analyticsService != nil {
		svc.logger.Debug("stopping analytics service")
		svc.analyticsService.Stop()
	}
	svc.logger.Debug("stopping profiling")
	svc.selfProfiling.Stop()
	svc.logger.Debug("stopping upstream")
	svc.directUpstream.Stop()
	svc.logger.Debug("stopping storage")
	if err := svc.storage.Close(); err != nil {
		svc.logger.WithError(err).Error("storage close")
	}

}
