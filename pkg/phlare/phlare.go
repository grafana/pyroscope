package phlare

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcutil"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/runtimeconfig"
	"github.com/grafana/dskit/services"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/samber/lo"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"github.com/weaveworks/common/signals"
	wwtracing "github.com/weaveworks/common/tracing"

	"github.com/grafana/phlare/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/phlare/pkg/agent"
	"github.com/grafana/phlare/pkg/api"
	"github.com/grafana/phlare/pkg/cfg"
	"github.com/grafana/phlare/pkg/distributor"
	"github.com/grafana/phlare/pkg/frontend"
	"github.com/grafana/phlare/pkg/ingester"
	"github.com/grafana/phlare/pkg/objstore"
	objstoreclient "github.com/grafana/phlare/pkg/objstore/client"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/querier"
	"github.com/grafana/phlare/pkg/querier/worker"
	"github.com/grafana/phlare/pkg/scheduler"
	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/tracing"
	"github.com/grafana/phlare/pkg/usagestats"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/validation"
	"github.com/grafana/phlare/pkg/validation/exporter"
)

type Config struct {
	Target            flagext.StringSliceCSV `yaml:"target,omitempty"`
	AgentConfig       agent.Config           `yaml:",inline"`
	API               api.Config             `yaml:"api"`
	Server            server.Config          `yaml:"server,omitempty"`
	Distributor       distributor.Config     `yaml:"distributor,omitempty"`
	Querier           querier.Config         `yaml:"querier,omitempty"`
	Frontend          frontend.Config        `yaml:"frontend,omitempty"`
	Worker            worker.Config          `yaml:"frontend_worker"`
	LimitsConfig      validation.Limits      `yaml:"limits"`
	QueryScheduler    scheduler.Config       `yaml:"query_scheduler"`
	Ingester          ingester.Config        `yaml:"ingester,omitempty"`
	MemberlistKV      memberlist.KVConfig    `yaml:"memberlist"`
	PhlareDB          phlaredb.Config        `yaml:"phlaredb,omitempty"`
	Tracing           tracing.Config         `yaml:"tracing"`
	OverridesExporter exporter.Config        `yaml:"overrides_exporter" doc:"hidden"`
	RuntimeConfig     runtimeconfig.Config   `yaml:"runtime_config"`

	Storage StorageConfig `yaml:"storage"`

	MultitenancyEnabled bool              `yaml:"multitenancy_enabled,omitempty"`
	Analytics           usagestats.Config `yaml:"analytics"`

	ConfigFile      string `yaml:"-"`
	ConfigExpandEnv bool   `yaml:"-"`
}

func newDefaultConfig() *Config {
	defaultConfig := &Config{}
	defaultFS := flag.NewFlagSet("", flag.PanicOnError)
	defaultConfig.RegisterFlags(defaultFS)
	return defaultConfig
}

type StorageConfig struct {
	Bucket objstoreclient.Config `yaml:",inline"`
}

func (c *StorageConfig) RegisterFlagsWithContext(ctx context.Context, f *flag.FlagSet) {
	c.Bucket.RegisterFlagsWithPrefix("storage.", f, phlarecontext.Logger(ctx))
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	c.RegisterFlagsWithContext(context.Background(), f)
}

// RegisterFlagsWithContext registers flag.
func (c *Config) RegisterFlagsWithContext(ctx context.Context, f *flag.FlagSet) {
	// Set the default module list to 'all'
	c.Target = []string{All}
	f.StringVar(&c.ConfigFile, "config.file", "", "yaml file to load")
	f.Var(&c.Target, "target", "Comma-separated list of Phlare modules to load. "+
		"The alias 'all' can be used in the list to load a number of core modules and will enable single-binary mode. ")
	f.BoolVar(&c.MultitenancyEnabled, "auth.multitenancy-enabled", false, "When set to true, incoming HTTP requests must specify tenant ID in HTTP X-Scope-OrgId header. When set to false, tenant ID anonymous is used instead.")
	f.BoolVar(&c.ConfigExpandEnv, "config.expand-env", false, "Expands ${var} in config according to the values of the environment variables.")

	c.registerServerFlagsWithChangedDefaultValues(f)
	c.AgentConfig.RegisterFlags(f)
	c.MemberlistKV.RegisterFlags(f)
	c.Querier.RegisterFlags(f)
	c.PhlareDB.RegisterFlags(f)
	c.Tracing.RegisterFlags(f)
	c.Storage.RegisterFlagsWithContext(ctx, f)
	c.RuntimeConfig.RegisterFlags(f)
	c.Analytics.RegisterFlags(f)
	c.LimitsConfig.RegisterFlags(f)
	c.API.RegisterFlags(f)
}

// registerServerFlagsWithChangedDefaultValues registers *Config.Server flags, but overrides some defaults set by the weaveworks package.
func (c *Config) registerServerFlagsWithChangedDefaultValues(fs *flag.FlagSet) {
	throwaway := flag.NewFlagSet("throwaway", flag.PanicOnError)

	// Register to throwaway flags first. Default values are remembered during registration and cannot be changed,
	// but we can take values from throwaway flag set and reregister into supplied flags with new default values.
	c.Server.RegisterFlags(throwaway)
	c.Ingester.RegisterFlags(throwaway)
	c.Distributor.RegisterFlags(throwaway)
	c.Frontend.RegisterFlags(throwaway, log.NewLogfmtLogger(os.Stderr))
	c.QueryScheduler.RegisterFlags(throwaway, log.NewLogfmtLogger(os.Stderr))
	c.Worker.RegisterFlags(throwaway)
	c.OverridesExporter.RegisterFlags(throwaway, log.NewLogfmtLogger(os.Stderr))

	throwaway.VisitAll(func(f *flag.Flag) {
		// Ignore errors when setting new values. We have a test to verify that it works.
		switch f.Name {
		case "server.http-listen-port":
			_ = f.Value.Set("4100")
		case "query-frontend.instance-port":
			_ = f.Value.Set("4100")
		case "distributor.ring.instance-port":
			_ = f.Value.Set("4100")
		case "overrides-exporter.ring.instance-port":
			_ = f.Value.Set("4100")
		case "distributor.replication-factor":
			_ = f.Value.Set("1")
		case "query-scheduler.service-discovery-mode":
			_ = f.Value.Set(schedulerdiscovery.ModeRing)
		}
		fs.Var(f.Value, f.Name, f.Usage)
	})
}

func (c *Config) Validate() error {
	if len(c.Target) == 0 {
		return errors.New("no modules specified")
	}
	if err := c.Ingester.Validate(); err != nil {
		return err
	}
	return c.AgentConfig.Validate()
}

type phlareConfigGetter interface {
	PhlareConfig() *Config
}

func (c *Config) ApplyDynamicConfig() cfg.Source {
	c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store = "memberlist"
	c.Distributor.DistributorRing.KVStore.Store = c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store
	c.OverridesExporter.Ring.KVStore.Store = c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store
	c.Frontend.QuerySchedulerDiscovery.SchedulerRing.KVStore.Store = c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store
	c.Worker.QuerySchedulerDiscovery.SchedulerRing.KVStore.Store = c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store
	c.QueryScheduler.ServiceDiscovery.SchedulerRing.KVStore.Store = c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store
	c.Worker.MaxConcurrentRequests = 4 // todo we might want this as a config flags.

	return func(dst cfg.Cloneable) error {
		g, ok := dst.(phlareConfigGetter)
		if !ok {
			return fmt.Errorf("dst is not a Phlare config getter %T", dst)
		}
		r := g.PhlareConfig()
		if r.AgentConfig.ClientConfig.URL.String() == "" {
			listenAddress := "0.0.0.0"
			if c.Server.HTTPListenAddress != "" {
				listenAddress = c.Server.HTTPListenAddress
			}

			if err := r.AgentConfig.ClientConfig.URL.Set(fmt.Sprintf("http://%s:%d", listenAddress, c.Server.HTTPListenPort)); err != nil {
				return err
			}
		}
		return nil
	}
}

func (c *Config) Clone() flagext.Registerer {
	return func(c Config) *Config {
		return &c
	}(*c)
}

type Phlare struct {
	Cfg    Config
	logger log.Logger
	reg    prometheus.Registerer
	tracer io.Closer

	ModuleManager *modules.Manager
	serviceMap    map[string]services.Service
	deps          map[string][]string

	API           *api.API
	Server        *server.Server
	SignalHandler *signals.Handler
	MemberlistKV  *memberlist.KVInitService
	ring          *ring.Ring
	agent         *agent.Agent
	pusherClient  pushv1connect.PusherServiceClient
	usageReport   *usagestats.Reporter
	RuntimeConfig *runtimeconfig.Manager
	Overrides     *validation.Overrides

	TenantLimits validation.TenantLimits

	storageBucket objstore.Bucket

	grpcGatewayMux *grpcgw.ServeMux

	auth connect.Option
}

func New(cfg Config) (*Phlare, error) {
	logger := initLogger(&cfg.Server)
	usagestats.Edition("oss")

	phlare := &Phlare{
		Cfg:    cfg,
		logger: logger,
		reg:    prometheus.DefaultRegisterer,
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := phlare.setupModuleManager(); err != nil {
		return nil, err
	}

	if cfg.Tracing.Enabled {
		// Setting the environment variable JAEGER_AGENT_HOST enables tracing
		trace, err := wwtracing.NewFromEnv(fmt.Sprintf("phlare-%s", cfg.Target))
		if err != nil {
			level.Error(logger).Log("msg", "error in initializing tracing. tracing will not be enabled", "err", err)
		}
		phlare.tracer = trace
	}

	// instantiate a fallback pusher client (when not run with a local distributor)
	pusherHTTPClient, err := commonconfig.NewClientFromConfig(cfg.AgentConfig.ClientConfig.Client, cfg.AgentConfig.ClientConfig.URL.String())
	if err != nil {
		return nil, err
	}
	phlare.auth = connect.WithInterceptors(tenant.NewAuthInterceptor(cfg.MultitenancyEnabled))

	pusherHTTPClient.Transport = util.WrapWithInstrumentedHTTPTransport(pusherHTTPClient.Transport)
	phlare.pusherClient = pushv1connect.NewPusherServiceClient(pusherHTTPClient,
		cfg.AgentConfig.ClientConfig.URL.String(),
		phlare.auth,
	)

	phlare.Cfg.API.HTTPAuthMiddleware = util.AuthenticateUser(cfg.MultitenancyEnabled)
	phlare.Cfg.API.GrpcAuthMiddleware = phlare.auth

	return phlare, nil
}

func (f *Phlare) setupModuleManager() error {
	mm := modules.NewManager(f.logger)

	mm.RegisterModule(Storage, f.initStorage, modules.UserInvisibleModule)
	mm.RegisterModule(GRPCGateway, f.initGRPCGateway, modules.UserInvisibleModule)
	mm.RegisterModule(MemberlistKV, f.initMemberlistKV, modules.UserInvisibleModule)
	mm.RegisterModule(Ring, f.initRing, modules.UserInvisibleModule)
	mm.RegisterModule(RuntimeConfig, f.initRuntimeConfig, modules.UserInvisibleModule)
	mm.RegisterModule(Overrides, f.initOverrides, modules.UserInvisibleModule)
	mm.RegisterModule(OverridesExporter, f.initOverridesExporter)
	mm.RegisterModule(Ingester, f.initIngester)
	mm.RegisterModule(Server, f.initServer, modules.UserInvisibleModule)
	mm.RegisterModule(API, f.initAPI, modules.UserInvisibleModule)
	mm.RegisterModule(Distributor, f.initDistributor)
	mm.RegisterModule(Querier, f.initQuerier)
	mm.RegisterModule(Agent, f.initAgent)
	mm.RegisterModule(UsageReport, f.initUsageReport)
	mm.RegisterModule(QueryFrontend, f.initQueryFrontend)
	mm.RegisterModule(QueryScheduler, f.initQueryScheduler)
	mm.RegisterModule(All, nil)

	// Add dependencies
	deps := map[string][]string{
		All: {Agent, Ingester, Distributor, QueryScheduler, QueryFrontend, Querier},

		Server:         {GRPCGateway},
		API:            {Server},
		Agent:          {API},
		Distributor:    {Overrides, Ring, API, UsageReport},
		Querier:        {API, MemberlistKV, Ring, UsageReport},
		QueryFrontend:  {OverridesExporter, API, MemberlistKV, UsageReport},
		QueryScheduler: {Overrides, API, MemberlistKV, UsageReport},
		Ingester:       {Overrides, API, MemberlistKV, Storage, UsageReport},

		UsageReport:       {Storage, MemberlistKV},
		Overrides:         {RuntimeConfig},
		OverridesExporter: {Overrides, MemberlistKV},
		RuntimeConfig:     {API},
		Ring:              {API, MemberlistKV},
		MemberlistKV:      {API},
	}

	for mod, targets := range deps {
		if err := mm.AddDependency(mod, targets...); err != nil {
			return err
		}
	}

	f.deps = deps
	f.ModuleManager = mm

	return nil
}

func (f *Phlare) Run() error {
	serviceMap, err := f.ModuleManager.InitModuleServices(f.Cfg.Target...)
	if err != nil {
		return err
	}

	f.serviceMap = serviceMap
	var servs []services.Service
	for _, s := range serviceMap {
		servs = append(servs, s)
	}

	sm, err := services.NewManager(servs...)
	if err != nil {
		return err
	}
	f.Server.HTTP.Path("/ready").Methods("GET").Handler(f.readyHandler(sm))

	RegisterHealthServer(f.Server.HTTP, grpcutil.WithManager(sm))
	healthy := func() { level.Info(f.logger).Log("msg", "Phlare started", "version", version.Info()) }

	serviceFailed := func(service services.Service) {
		// if any service fails, stop entire Phlare
		sm.StopAsync()

		// let's find out which module failed
		for m, s := range serviceMap {
			if s == service {
				if service.FailureCase() == modules.ErrStopProcess {
					level.Info(f.logger).Log("msg", "received stop signal via return error", "module", m, "error", service.FailureCase())
				} else {
					level.Error(f.logger).Log("msg", "module failed", "module", m, "error", service.FailureCase())
				}
				return
			}
		}

		level.Error(f.logger).Log("msg", "module failed", "module", "unknown", "error", service.FailureCase())
	}

	sm.AddListener(services.NewManagerListener(healthy, f.stopped, serviceFailed))

	// Setup signal handler. If signal arrives, we stop the manager, which stops all the services.
	f.SignalHandler = signals.NewHandler(f.Server.Log)
	go func() {
		f.SignalHandler.Loop()
		sm.StopAsync()
	}()

	// Start all services. This can really only fail if some service is already
	// in other state than New, which should not be the case.
	err = sm.StartAsync(context.Background())
	if err == nil {
		// Wait until service manager stops. It can stop in two ways:
		// 1) Signal is received and manager is stopped.
		// 2) Any service fails.
		err = sm.AwaitStopped(context.Background())
	}

	// If there is no error yet (= service manager started and then stopped without problems),
	// but any service failed, report that failure as an error to caller.
	if err == nil {
		if failed := sm.ServicesByState()[services.Failed]; len(failed) > 0 {
			for _, f := range failed {
				if f.FailureCase() != modules.ErrStopProcess {
					// Details were reported via failure listener before
					err = errors.New("failed services")
					break
				}
			}
		}
	}
	return err
}

func (f *Phlare) readyHandler(sm *services.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sm.IsHealthy() {
			msg := bytes.Buffer{}
			msg.WriteString("Some services are not Running:\n")

			byState := map[services.State][]string{}
			for name, svc := range f.serviceMap {
				state := svc.State()
				byState[state] = append(byState[state], name)
			}

			states := lo.Keys(byState)
			sort.Slice(states, func(i, j int) bool { return states[i] < states[j] })

			for _, st := range states {
				sort.Strings(byState[st])
				msg.WriteString(fmt.Sprintf("%v: %v\n", st, byState[st]))
			}

			http.Error(w, msg.String(), http.StatusServiceUnavailable)
			return
		}

		util.WriteTextResponse(w, "ready")
	}
}

func (f *Phlare) stopped() {
	level.Info(f.logger).Log("msg", "Phlare stopped")
	if f.tracer != nil {
		if err := f.tracer.Close(); err != nil {
			level.Error(f.logger).Log("msg", "error closing tracing", "err", err)
		}
	}
}

func initLogger(cfg *server.Config) log.Logger {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	if cfg.LogFormat.String() == "json" {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	}
	logger = level.NewFilter(logger, levelFilter(cfg.LogLevel.String()))

	// when use util_log.Logger, skip 3 stack frames.
	logger = log.With(logger, "caller", log.Caller(3))

	// cfg.Log wraps log function, skip 4 stack frames to get caller information.
	// this works in go 1.12, but doesn't work in versions earlier.
	// it will always shows the wrapper function generated by compiler
	// marked <autogenerated> in old versions.
	cfg.Log = logging.GoKit(log.With(logger, "caller", log.Caller(4)))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	util.Logger = logger
	return logger
}

func (f *Phlare) initAPI() (services.Service, error) {
	a, err := api.New(f.Cfg.API, f.Server, f.grpcGatewayMux, util.Logger)
	if err != nil {
		return nil, err
	}
	f.API = a

	if err := f.API.RegisterAPI(f.statusService()); err != nil {
		return nil, err
	}

	return nil, nil
}

func levelFilter(l string) level.Option {
	switch l {
	case "debug":
		return level.AllowDebug()
	case "info":
		return level.AllowInfo()
	case "warn":
		return level.AllowWarn()
	case "error":
		return level.AllowError()
	default:
		return level.AllowAll()
	}
}
