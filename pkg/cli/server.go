package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/scrape"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
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
	discoveryManager *discovery.Manager
	scrapeManager    *scrape.Manager

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

	svc.discoveryManager = discovery.NewManager(
		svc.logger.WithField("component", "discovery-manager"))

	svc.scrapeManager = scrape.NewManager(
		svc.logger.WithField("component", "scrape-manager"),
		svc.directUpstream)

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

	// Scrape and Discovery managers have to be initialized
	// with ApplyConfig before starting running.
	if err := svc.discoveryManager.ApplyConfig(discoveryConfigs(svc.config.ScrapeConfigs)); err != nil {
		return err
	}
	if err := svc.scrapeManager.ApplyConfig(svc.config.ScrapeConfigs); err != nil {
		return err
	}
	g.Go(func() error {
		svc.logger.Debug("starting discovery manager")
		return svc.discoveryManager.Run()
	})
	g.Go(func() error {
		svc.logger.Debug("starting scrape manager")
		return svc.scrapeManager.Run(svc.discoveryManager.SyncCh())
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
	svc.logger.Debug("stopping discovery manager")
	svc.discoveryManager.Stop()
	svc.logger.Debug("stopping scrape manager")
	svc.scrapeManager.Stop()
	svc.logger.Debug("stopping storage")
	if err := svc.storage.Close(); err != nil {
		svc.logger.WithError(err).Error("storage close")
	}
	svc.logger.Debug("stopping http server")
	if err := svc.controller.Stop(); err != nil {
		svc.logger.WithError(err).Error("controller stop")
	}
}

func discoveryConfigs(cfg []*scrape.Config) map[string]discovery.Configs {
	c := make(map[string]discovery.Configs)
	for _, x := range cfg {
		c[x.JobName] = x.ServiceDiscoveryConfigs
	}
	return c
}

func loadServerConfig(c *config.Server) error {
	b, err := os.ReadFile(c.Config)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return nil
	default:
		return err
	}
	var s config.Server
	if err = yaml.Unmarshal(b, &s); err != nil {
		return err
	}

	// Populate scrape configs.
	c.ScrapeConfigs = s.ScrapeConfigs
	return nil
}
