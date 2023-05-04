package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/client/pyroscope"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"

	// revive:disable:blank-imports register discoverer
	"github.com/pyroscope-io/pyroscope/pkg/baseurl"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/aws"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/consul"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/file"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/http"
	_ "github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/kubernetes"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/chstore"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/scrape"
	sc "github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
	"github.com/pyroscope-io/pyroscope/pkg/selfprofiling"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/service"

	// "github.com/pyroscope-io/pyroscope/pkg/sqlstore"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/debug"
)

type Server struct {
	svc *serverService
}

type serverService struct {
	config     *config.Server
	logger     *logrus.Logger
	controller *server.Controller
	storage    *storage.Storage
	// queue used to ingest data into the storage
	storageQueue     *storage.IngestionQueue
	analyticsService *analytics.Service
	selfProfiling    *pyroscope.Session
	debugReporter    *debug.Reporter
	healthController *health.Controller
	adminServer      *admin.Server
	discoveryManager *discovery.Manager
	scrapeManager    *scrape.Manager
	// database         *sqlstore.SQLStore
	database         *chstore.CHStore
	remoteWriteQueue []*remotewrite.IngestionQueue

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

	diskPressure := health.DiskPressure{
		Threshold: c.MinFreeSpacePercentage,
		Path:      c.StoragePath,
	}

	svc.database, err = chstore.Open(c)
	// svc.database, err = sqlstore.Open(c)
	if err != nil {
		return nil, fmt.Errorf("can't open database %q: %w", c.Database.URL, err)
	}

	svc.healthController = health.NewController(svc.logger, time.Minute, diskPressure)

	fmt.Println("metadata saver keval")

	var appMetadataSaver storage.ApplicationMetadataSaver = service.NewApplicationMetadataService(svc.database.DB())
	appMetadataSaver = service.NewApplicationMetadataCacheService(service.ApplicationMetadataCacheServiceConfig{}, appMetadataSaver)

	storageConfig := storage.NewConfig(svc.config)
	svc.storage, err = storage.New(storageConfig, svc.logger, prometheus.DefaultRegisterer, svc.healthController, appMetadataSaver)
	if err != nil {
		return nil, fmt.Errorf("new storage: %w", err)
	}

	svc.debugReporter = debug.NewReporter(svc.logger, svc.storage, prometheus.DefaultRegisterer)

	if svc.config.Auth.JWTSecret == "" {
		if svc.config.Auth.JWTSecret, err = svc.storage.JWT(); err != nil {
			return nil, err
		}
	}

	appMetadataSvc := service.NewApplicationMetadataService(svc.database.DB())
	migrator := NewAppMetadataMigrator(logger, svc.storage, appMetadataSvc)
	err = migrator.Migrate()
	if err != nil {
		svc.logger.Error(err)
	}
	appSvc := service.NewApplicationService(appMetadataSvc, svc.storage)

	// this needs to happen after storage is initiated!
	if svc.config.EnableExperimentalAdmin {
		socketPath := svc.config.AdminSocketPath
		userService := service.NewUserService(svc.database.DB())
		adminController := admin.NewController(svc.logger, appSvc, userService, svc.storage)
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
			adminController,
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

	svc.storageQueue = storage.NewIngestionQueue(svc.logger, svc.storage, prometheus.DefaultRegisterer, storageConfig)

	defaultMetricsRegistry := prometheus.DefaultRegisterer

	var ingester ingestion.Ingester
	if !svc.config.RemoteWrite.Enabled || !svc.config.RemoteWrite.DisableLocalWrites {
		fmt.Println("if keval")
		ingester = parser.New(svc.logger, svc.storageQueue, metricsExporter)
	} else {
		fmt.Println("else keval")
		ingester = ingestion.NewNoopIngester()
	}

	// ingester = ingestion.NewMiddlewareIngester()

	// If remote write is available, let's write to both local storage and to the remote server
	if svc.config.RemoteWrite.Enabled {

		fmt.Println("\nremote write enabled keval\n")

		err = loadRemoteWriteTargetConfigsFromFile(svc.config)
		if err != nil {
			return nil, err
		}

		if len(svc.config.RemoteWrite.Targets) <= 0 {
			return nil, fmt.Errorf("remote write is enabled but no targets are set up")
		}

		remoteClients := make([]ingestion.Ingester, len(svc.config.RemoteWrite.Targets))
		svc.remoteWriteQueue = make([]*remotewrite.IngestionQueue, len(svc.config.RemoteWrite.Targets))

		i := 0
		for targetName, t := range svc.config.RemoteWrite.Targets {
			targetLogger := logger.WithField("remote_target", targetName)
			targetLogger.Debug("Initializing remote write target")

			remoteClient := remotewrite.NewClient(targetLogger, defaultMetricsRegistry, targetName, t)
			q := remotewrite.NewIngestionQueue(targetLogger, defaultMetricsRegistry, remoteClient, targetName, t)

			remoteClients[i] = q
			svc.remoteWriteQueue[i] = q
			i++
		}

		ingesters := append([]ingestion.Ingester{ingester}, remoteClients...)
		ingester = ingestion.NewParallelizer(svc.logger, ingesters...)
	}
	if !svc.config.NoSelfProfiling {
		svc.selfProfiling = selfprofiling.NewSession(svc.logger, ingester, "pyroscope.server", svc.config.SelfProfilingTags)
	}

	svc.scrapeManager = scrape.NewManager(
		svc.logger.WithField("component", "scrape-manager"),
		ingester,
		defaultMetricsRegistry)

	svc.controller, err = server.New(server.Config{
		Configuration:           svc.config,
		Storage:                 svc.storage,
		Ingester:                ingester,
		Notifier:                svc.healthController,
		Logger:                  svc.logger,
		MetricsRegisterer:       defaultMetricsRegistry,
		ExportedMetricsRegistry: exportedMetricsRegistry,
		ScrapeManager:           svc.scrapeManager,
		DB:                      svc.database.DB(),
	})
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}

	svc.discoveryManager = discovery.NewManager(
		svc.logger.WithField("component", "discovery-manager"))

	if !c.AnalyticsOptOut {
		svc.analyticsService = analytics.NewService(c, svc.storage, svc.controller)
	}

	if os.Getenv("PYROSCOPE_CONFIG_DEBUG") != "" {
		fmt.Println("parsed config:")
		spew.Dump(svc.config)
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

	if svc.config.BaseURLBindAddr != "" {
		go baseurl.Start(svc.config)
	}
	go svc.debugReporter.Start()
	if svc.analyticsService != nil {
		go svc.analyticsService.Start()
	}

	svc.healthController.Start()
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

	if svc.config.RemoteWrite.Enabled {
		svc.logger.Debug("stopping remote queues")
		for _, q := range svc.remoteWriteQueue {
			q.Stop()
		}
	}

	svc.logger.Debug("stopping ingestion queue")
	svc.storageQueue.Stop()
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
	type scrapeConfig struct {
		ScrapeConfigs []*sc.Config `yaml:"scrape-configs" mapstructure:"-"`
	}
	var s scrapeConfig
	if err = yaml.Unmarshal([]byte(performSubstitutions(b)), &s); err != nil {
		return err
	}
	// Populate scrape configs.
	c.ScrapeConfigs = s.ScrapeConfigs
	return nil
}

func loadRemoteWriteTargetConfigsFromFile(c *config.Server) error {
	b, err := os.ReadFile(c.Config)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return nil
	default:
		return err
	}

	type cfg struct {
		RemoteWrite struct {
			Targets map[string]config.RemoteWriteTarget `yaml:"targets" mapstructure:"-"`
		} `yaml:"remote-write"`
	}

	var s cfg
	if err = yaml.Unmarshal([]byte(performSubstitutions(b)), &s); err != nil {
		return err
	}

	c.RemoteWrite.Targets = s.RemoteWrite.Targets

	return nil
}
