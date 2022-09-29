package fire

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcutil"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/middleware"
	"github.com/weaveworks/common/server"
	"github.com/weaveworks/common/signals"
	wwtracing "github.com/weaveworks/common/tracing"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/fire/pkg/agent"
	"github.com/grafana/fire/pkg/cfg"
	"github.com/grafana/fire/pkg/distributor"
	"github.com/grafana/fire/pkg/firedb"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"
	"github.com/grafana/fire/pkg/ingester"
	"github.com/grafana/fire/pkg/objstore"
	"github.com/grafana/fire/pkg/querier"
	"github.com/grafana/fire/pkg/tenant"
	"github.com/grafana/fire/pkg/tracing"
	"github.com/grafana/fire/pkg/util"
)

type Config struct {
	Target       flagext.StringSliceCSV `yaml:"target,omitempty"`
	AgentConfig  agent.Config           `yaml:",inline"`
	Server       server.Config          `yaml:"server,omitempty"`
	Distributor  distributor.Config     `yaml:"distributor,omitempty"`
	Querier      querier.Config         `yaml:"querier,omitempty"`
	Ingester     ingester.Config        `yaml:"ingester,omitempty"`
	MemberlistKV memberlist.KVConfig    `yaml:"memberlist"`
	FireDB       firedb.Config          `yaml:"firedb,omitempty"`
	Tracing      tracing.Config         `yaml:"tracing"`

	Storage StorageConfig `yaml:"storage"`

	MultitenancyEnabled bool `yaml:"multitenancy_enabled,omitempty"`
	ConfigFile          string
}

type StorageConfig struct {
	// TODO: This is probably to simple and will need needs replacing
	BucketConfig string `yaml:"bucketConfig,omitempty"`
}

// RegisterFlags registers flag.
func (c *Config) RegisterFlags(f *flag.FlagSet) {
	// Set the default module list to 'all'
	c.Target = []string{All}
	f.StringVar(&c.ConfigFile, "config.file", "", "yaml file to load")
	f.Var(&c.Target, "target", "Comma-separated list of Fire modules to load. "+
		"The alias 'all' can be used in the list to load a number of core modules and will enable single-binary mode. ")
	f.BoolVar(&c.MultitenancyEnabled, "auth.multitenancy-enabled", false, "When set to true, incoming HTTP requests must specify tenant ID in HTTP X-Scope-OrgId header. When set to false, tenant ID anonymous is used instead.")

	c.registerServerFlagsWithChangedDefaultValues(f)
	c.AgentConfig.RegisterFlags(f)
	c.MemberlistKV.RegisterFlags(f)
	c.Distributor.RegisterFlags(f)
	c.Querier.RegisterFlags(f)
	c.FireDB.RegisterFlags(f)
	c.Tracing.RegisterFlags(f)
}

// registerServerFlagsWithChangedDefaultValues registers *Config.Server flags, but overrides some defaults set by the weaveworks package.
func (c *Config) registerServerFlagsWithChangedDefaultValues(fs *flag.FlagSet) {
	throwaway := flag.NewFlagSet("throwaway", flag.PanicOnError)

	// Register to throwaway flags first. Default values are remembered during registration and cannot be changed,
	// but we can take values from throwaway flag set and reregister into supplied flags with new default values.
	c.Server.RegisterFlags(throwaway)
	c.Ingester.RegisterFlags(throwaway)

	throwaway.VisitAll(func(f *flag.Flag) {
		// Ignore errors when setting new values. We have a test to verify that it works.
		switch f.Name {
		case "server.http-listen-port":
			_ = f.Value.Set("4100")
		case "distributor.replication-factor":
			_ = f.Value.Set("1")
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

func (c *Config) ApplyDynamicConfig() cfg.Source {
	c.Ingester.LifecyclerConfig.RingConfig.KVStore.Store = "memberlist"
	return func(dst cfg.Cloneable) error {
		r, ok := dst.(*Config)
		if !ok {
			return errors.New("dst is not a Fire config")
		}
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

type Fire struct {
	Cfg    Config
	logger log.Logger
	reg    prometheus.Registerer
	tracer io.Closer

	ModuleManager *modules.Manager
	serviceMap    map[string]services.Service
	deps          map[string][]string

	HTTPAuthMiddleware middleware.Interface
	Server             *server.Server
	SignalHandler      *signals.Handler
	MemberlistKV       *memberlist.KVInitService
	ring               *ring.Ring
	agent              *agent.Agent
	pusherClient       pushv1connect.PusherServiceClient

	storageBucket objstore.Bucket

	grpcGatewayMux *grpcgw.ServeMux

	auth connect.Option
}

func New(cfg Config) (*Fire, error) {
	logger := initLogger(&cfg.Server)

	fire := &Fire{
		Cfg:    cfg,
		logger: logger,
		reg:    prometheus.DefaultRegisterer,
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := fire.setupModuleManager(); err != nil {
		return nil, err
	}

	if cfg.Tracing.Enabled {
		// Setting the environment variable JAEGER_AGENT_HOST enables tracing
		trace, err := wwtracing.NewFromEnv(fmt.Sprintf("fire-%s", cfg.Target))
		if err != nil {
			level.Error(logger).Log("msg", "error in initializing tracing. tracing will not be enabled", "err", err)
		}
		fire.tracer = trace
	}

	// instantiate a fallback pusher client (when not run with a local distributor
	pusherHTTPClient, err := commonconfig.NewClientFromConfig(cfg.AgentConfig.ClientConfig.Client, cfg.AgentConfig.ClientConfig.URL.String())
	if err != nil {
		return nil, err
	}
	fire.auth = connect.WithInterceptors(tenant.NewAuthInterceptor(cfg.MultitenancyEnabled))

	pusherHTTPClient.Transport = util.WrapWithInstrumentedHTTPTransport(pusherHTTPClient.Transport)
	fire.pusherClient = pushv1connect.NewPusherServiceClient(pusherHTTPClient,
		cfg.AgentConfig.ClientConfig.URL.String(),
		fire.auth,
	)
	return fire, nil
}

func (f *Fire) setupModuleManager() error {
	mm := modules.NewManager(f.logger)

	mm.RegisterModule(Storage, f.initStorage, modules.UserInvisibleModule)
	mm.RegisterModule(GRPCGateway, f.initGRPCGateway, modules.UserInvisibleModule)
	mm.RegisterModule(MemberlistKV, f.initMemberlistKV, modules.UserInvisibleModule)
	mm.RegisterModule(Ring, f.initRing, modules.UserInvisibleModule)
	mm.RegisterModule(Ingester, f.initIngester)
	mm.RegisterModule(Server, f.initServer, modules.UserInvisibleModule)
	mm.RegisterModule(Distributor, f.initDistributor)
	mm.RegisterModule(Querier, f.initQuerier)
	mm.RegisterModule(Agent, f.initAgent)
	mm.RegisterModule(All, nil)

	// Add dependencies
	deps := map[string][]string{
		All:          {Agent, Ingester, Distributor, Querier},
		Distributor:  {Ring, Server},
		Querier:      {Ring, Server},
		Agent:        {Server, GRPCGateway},
		Ingester:     {Server, MemberlistKV, Storage},
		Ring:         {Server, MemberlistKV},
		MemberlistKV: {Server},
		GRPCGateway:  {Server},

		// Querier:                  {Store, Ring, Server, IngesterQuerier, TenantConfigs, UsageReport},
		// QueryFrontendTripperware: {Server, Overrides, TenantConfigs},
		// QueryFrontend:            {QueryFrontendTripperware, UsageReport},
		// QueryScheduler:           {Server, Overrides, MemberlistKV, UsageReport},
		// Ruler:                    {Ring, Server, Store, RulerStorage, IngesterQuerier, Overrides, TenantConfigs, UsageReport},
		// TableManager:             {Server, UsageReport},
		// Compactor:                {Server, Overrides, MemberlistKV, UsageReport},
		// IndexGateway:             {Server, Store, Overrides, UsageReport, MemberlistKV, IndexGatewayRing},
		// IngesterQuerier:          {Ring},
		// IndexGatewayRing:         {RuntimeConfig, Server, MemberlistKV},
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

func (f *Fire) Run() error {
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

	grpc_health_v1.RegisterHealthServer(f.Server.GRPC, grpcutil.NewHealthCheck(sm))
	healthy := func() { level.Info(f.logger).Log("msg", "Fire started", "version", version.Info()) }

	serviceFailed := func(service services.Service) {
		// if any service fails, stop entire Fire
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

func (f *Fire) readyHandler(sm *services.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sm.IsHealthy() {
			msg := bytes.Buffer{}
			msg.WriteString("Some services are not Running:\n")

			byState := sm.ServicesByState()
			for st, ls := range byState {
				msg.WriteString(fmt.Sprintf("%v: %d\n", st, len(ls)))
			}

			http.Error(w, msg.String(), http.StatusServiceUnavailable)
			return
		}

		http.Error(w, "ready", http.StatusOK)
	}
}

func (f *Fire) stopped() {
	level.Info(f.logger).Log("msg", "Fire stopped")
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
