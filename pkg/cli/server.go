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

	// revive:disable:blank-imports register discoverer
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/file"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/kubernetes"

	adhocserver "github.com/pyroscope-io/pyroscope/pkg/adhoc/server"
	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/scrape"
	sc "github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/sqlstore"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/debug"
)

type Server struct {
	svc *serverService
}

type serverService struct {
	config               *config.Server
	logger               *logrus.Logger
	controller           *server.Controller
	storage              *storage.Storage
	directUpstream       *direct.Direct
	directScrapeUpstream *direct.Direct
	analyticsService     *analytics.Service
	selfProfiling        *agent.ProfileSession
	debugReporter        *debug.Reporter
	healthController     *health.Controller
	adminServer          *admin.Server
	discoveryManager     *discovery.Manager
	scrapeManager        *scrape.Manager
	database             *sqlstore.SQLStore

	stopped chan struct{}
	done    chan struct{}
	group   *errgroup.Group
}

func newServerService(c *config.Server) (*serverService, error) {
	logLevel, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return nil, err
	}

	logger := logrus.StandardLogger()
	logger.SetLevel(logLevel)

	if err = loadScrapeConfigsFromFile(c); err != nil {
		return nil, fmt.Errorf("could not load scrape config: %w", err)
	}

	svc := serverService{
		config:  c,
		logger:  logger,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}

	svc.storage, err = storage.New(storage.NewConfig(svc.config), svc.logger, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, fmt.Errorf("new storage: %w", err)
	}

	if svc.config.Auth.JWTSecret == "" {
		if svc.config.Auth.JWTSecret, err = svc.storage.JWT(); err != nil {
			return nil, err
		}
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
	svc.directScrapeUpstream = direct.New(svc.storage, metricsExporter)

	if !svc.config.NoSelfProfiling {
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

	svc.database, err = sqlstore.Open(c.Database)
	if err != nil {
		return nil, fmt.Errorf("can't open database %q: %w", c.Database.URL, err)
	}

	// TODO(kolesnikovae): DB seeding

	defaultMetricsRegistry := prometheus.DefaultRegisterer
	svc.controller, err = server.New(server.Config{
		Configuration:   svc.config,
		Storage:         svc.storage,
		MetricsExporter: metricsExporter,
		Notifier:        svc.healthController,
		Adhoc: adhocserver.New(
			svc.logger,
			svc.config.AdhocDataPath,
			svc.config.MaxNodesRender,
			svc.config.EnableExperimentalAdhocUI,
		),
		Logger:                  svc.logger,
		MetricsRegisterer:       defaultMetricsRegistry,
		ExportedMetricsRegistry: exportedMetricsRegistry,
		DB:                      svc.database.DB(),
	})
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}

	svc.discoveryManager = discovery.NewManager(
		svc.logger.WithField("component", "discovery-manager"))

	svc.scrapeManager = scrape.NewManager(
		svc.logger.WithField("component", "scrape-manager"),
		svc.storage,
		defaultMetricsRegistry)

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
	if svc.config.EnableExperimentalAdmin {
		g.Go(func() error {
			svc.logger.Info("starting admin server")
			return svc.adminServer.Start()
		})
	}

	go svc.debugReporter.Start()
	if svc.analyticsService != nil {
		go svc.analyticsService.Start()
	}

	svc.healthController.Start()
	svc.directUpstream.Start()
	svc.directScrapeUpstream.Start()

	if !svc.config.NoSelfProfiling {
		if err := svc.selfProfiling.Start(); err != nil {
			svc.logger.WithError(err).Error("failed to start self-profiling")
		}
	}

	// Scrape and Discovery managers have to be initialized
	// with ApplyConfig before starting running.
	if err := svc.applyScrapeConfigs(svc.config); err != nil {
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
		if err := svc.adminServer.Stop(); err != nil {
			svc.logger.WithError(err).Error("admin server stop")
		}
	}
	svc.controller.Drain()
	svc.logger.Debug("stopping discovery manager")
	svc.discoveryManager.Stop()
	svc.logger.Debug("stopping scrape manager")
	svc.scrapeManager.Stop()
	svc.logger.Debug("stopping debug reporter")
	svc.debugReporter.Stop()
	svc.healthController.Stop()
	if svc.analyticsService != nil {
		svc.logger.Debug("stopping analytics service")
		svc.analyticsService.Stop()
	}

	if !svc.config.NoSelfProfiling {
		svc.logger.Debug("stopping self profiling")
		svc.selfProfiling.Stop()
	}

	svc.logger.Debug("stopping upstream")
	svc.directUpstream.Stop()
	svc.directScrapeUpstream.Stop()
	svc.logger.Debug("stopping storage")
	if err := svc.storage.Close(); err != nil {
		svc.logger.WithError(err).Error("storage close")
	}
	svc.logger.Debug("closing database")
	if err := svc.database.Close(); err != nil {
		svc.logger.WithError(err).Error("database close")
	}
	// we stop the http server as the last thing due to:
	// 1. we may still want to bserve metric values while storage is closing
	// 2. we want the /healthz endpoint to still be responding while server is shutting down
	// (we are thinking in a k8s context here, but maybe 'terminationGracePeriodSeconds' makes this unnecessary)
	svc.logger.Debug("stopping http server")
	if err := svc.controller.Stop(); err != nil {
		svc.logger.WithError(err).Error("controller stop")
	}
}

func (svc *serverService) ApplyConfig(c *config.Server) error {
	return svc.applyScrapeConfigs(c)
}

func (svc *serverService) applyScrapeConfigs(c *config.Server) error {
	if err := loadScrapeConfigsFromFile(c); err != nil {
		return fmt.Errorf("could not load scrape configs from %s: %w", c.Config, err)
	}
	if err := svc.discoveryManager.ApplyConfig(discoveryConfigs(c.ScrapeConfigs)); err != nil {
		// discoveryManager.ApplyConfig never return errors.
		return err
	}
	return svc.scrapeManager.ApplyConfig(c.ScrapeConfigs)
}

func discoveryConfigs(cfg []*sc.Config) map[string]discovery.Configs {
	c := make(map[string]discovery.Configs)
	for _, x := range cfg {
		c[x.JobName] = x.ServiceDiscoveryConfigs
	}
	return c
}

func loadScrapeConfigsFromFile(c *config.Server) error {
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
