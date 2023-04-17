package phlare

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/felixge/fgprof"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/runtimeconfig"
	"github.com/grafana/dskit/services"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/version"
	"github.com/thanos-io/thanos/pkg/discovery/dns"
	"github.com/weaveworks/common/middleware"
	"github.com/weaveworks/common/server"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v2"

	"github.com/grafana/phlare/public"

	agentv1 "github.com/grafana/phlare/api/gen/proto/go/agent/v1"
	"github.com/grafana/phlare/api/gen/proto/go/agent/v1/agentv1connect"
	"github.com/grafana/phlare/api/gen/proto/go/ingester/v1/ingesterv1connect"
	"github.com/grafana/phlare/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	statusv1 "github.com/grafana/phlare/api/gen/proto/go/status/v1"
	"github.com/grafana/phlare/api/openapiv2"
	"github.com/grafana/phlare/pkg/agent"
	"github.com/grafana/phlare/pkg/api"
	"github.com/grafana/phlare/pkg/distributor"
	"github.com/grafana/phlare/pkg/frontend"
	"github.com/grafana/phlare/pkg/frontend/frontendpb/frontendpbconnect"
	"github.com/grafana/phlare/pkg/ingester"
	"github.com/grafana/phlare/pkg/ingester/pyroscope"
	objstoreclient "github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/querier"
	"github.com/grafana/phlare/pkg/querier/worker"
	"github.com/grafana/phlare/pkg/scheduler"
	"github.com/grafana/phlare/pkg/scheduler/schedulerpb/schedulerpbconnect"
	"github.com/grafana/phlare/pkg/usagestats"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/build"
	"github.com/grafana/phlare/pkg/validation"
	"github.com/grafana/phlare/pkg/validation/exporter"
)

// The various modules that make up Phlare.
const (
	All               string = "all"
	Agent             string = "agent"
	Distributor       string = "distributor"
	Server            string = "server"
	Ring              string = "ring"
	Ingester          string = "ingester"
	MemberlistKV      string = "memberlist-kv"
	Querier           string = "querier"
	GRPCGateway       string = "grpc-gateway"
	Storage           string = "storage"
	UsageReport       string = "usage-stats"
	QueryFrontend     string = "query-frontend"
	QueryScheduler    string = "query-scheduler"
	RuntimeConfig     string = "runtime-config"
	Overrides         string = "overrides"
	OverridesExporter string = "overrides-exporter"

	// QueryFrontendTripperware string = "query-frontend-tripperware"
	// Compactor                string = "compactor"
	// IndexGateway             string = "index-gateway"
	// IndexGatewayRing         string = "index-gateway-ring"
)

var objectStoreTypeStats = usagestats.NewString("store_object_type")

func (f *Phlare) initQueryFrontend() (services.Service, error) {
	if f.Cfg.Frontend.Addr == "" {
		addr, err := util.GetFirstAddressOf(f.Cfg.Frontend.InfNames)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get frontend address")
		}

		f.Cfg.Frontend.Addr = addr
	}

	if f.Cfg.Frontend.Port == 0 {
		f.Cfg.Frontend.Port = f.Cfg.Server.HTTPListenPort
	}

	frontendSvc, err := frontend.NewFrontend(f.Cfg.Frontend, log.With(f.logger, "component", "frontend"), f.reg)
	if err != nil {
		return nil, err
	}
	f.registerQuerierHandlers(querier.NewGRPCRoundTripper(frontendSvc))
	frontendpbconnect.RegisterFrontendForQuerierHandler(f.Server.HTTP, frontendSvc, f.auth)
	return frontendSvc, nil
}

func (f *Phlare) initRuntimeConfig() (services.Service, error) {
	if len(f.Cfg.RuntimeConfig.LoadPath) == 0 {
		// no need to initialize module if load path is empty
		return nil, nil
	}

	f.Cfg.RuntimeConfig.Loader = loadRuntimeConfig

	// make sure to set default limits before we start loading configuration into memory
	validation.SetDefaultLimitsForYAMLUnmarshalling(f.Cfg.LimitsConfig)

	serv, err := runtimeconfig.New(f.Cfg.RuntimeConfig, prometheus.WrapRegistererWithPrefix("phlare_", f.reg), log.With(f.logger, "component", "runtime-config"))
	if err == nil {
		// TenantLimits just delegates to RuntimeConfig and doesn't have any state or need to do
		// anything in the start/stopping phase. Thus we can create it as part of runtime config
		// setup without any service instance of its own.
		f.TenantLimits = newTenantLimits(serv)
	}

	f.RuntimeConfig = serv

	f.Server.HTTP.Methods("GET").Path("/runtime_config").Handler(runtimeConfigHandler(f.RuntimeConfig, f.Cfg.LimitsConfig))
	f.Server.HTTP.Methods("GET").Path("/api/v1/tenant_limits").Handler(middleware.AuthenticateUser.Wrap(validation.TenantLimitsHandler(f.Cfg.LimitsConfig, f.TenantLimits)))
	f.IndexPage.AddLinks(api.RuntimeConfigWeight, "Current runtime config", []api.IndexPageLink{
		{Desc: "Entire runtime config (including overrides)", Path: "/runtime_config"},
		{Desc: "Only values that differ from the defaults", Path: "/runtime_config?mode=diff"},
	})
	return serv, err
}

func (f *Phlare) initOverrides() (serv services.Service, err error) {
	f.Overrides, err = validation.NewOverrides(f.Cfg.LimitsConfig, f.TenantLimits)
	// overrides don't have operational state, nor do they need to do anything more in starting/stopping phase,
	// so there is no need to return any service.
	return nil, err
}

func (f *Phlare) initOverridesExporter() (services.Service, error) {
	overridesExporter, err := exporter.NewOverridesExporter(
		f.Cfg.OverridesExporter,
		&f.Cfg.LimitsConfig,
		f.TenantLimits,
		log.With(f.logger, "component", "overrides-exporter"),
		f.reg,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to instantiate overrides-exporter")
	}
	if f.reg != nil {
		f.reg.MustRegister(overridesExporter)
	}

	f.Server.HTTP.Methods("GET", "POST").Path("/overrides-exporter/ring").HandlerFunc(overridesExporter.RingHandler)
	f.IndexPage.AddLinks(api.DefaultWeight, "Overrides-exporter", []api.IndexPageLink{
		{Desc: "Ring status", Path: "/overrides-exporter/ring"},
	})
	return overridesExporter, nil
}

func (f *Phlare) initQueryScheduler() (services.Service, error) {
	f.Cfg.QueryScheduler.ServiceDiscovery.SchedulerRing.ListenPort = f.Cfg.Server.HTTPListenPort

	s, err := scheduler.NewScheduler(f.Cfg.QueryScheduler, f.Overrides, log.With(f.logger, "component", "scheduler"), f.reg)
	if err != nil {
		return nil, errors.Wrap(err, "query-scheduler init")
	}
	schedulerpbconnect.RegisterSchedulerForFrontendHandler(f.Server.HTTP, s)
	schedulerpbconnect.RegisterSchedulerForQuerierHandler(f.Server.HTTP, s)
	return s, nil
}

// setupWorkerTimeout sets the max loop duration for the querier worker and frontend worker
// to 90% of the read or write http timeout, whichever is smaller.
// This is to ensure that the worker doesn't timeout before the http handler and that the connection
// is refreshed.
func (f *Phlare) setupWorkerTimeout() {
	timeout := f.Cfg.Server.HTTPServerReadTimeout
	if f.Cfg.Server.HTTPServerWriteTimeout < timeout {
		timeout = f.Cfg.Server.HTTPServerWriteTimeout
	}

	if timeout > 0 {
		f.Cfg.Worker.MaxLoopDuration = time.Duration(float64(timeout) * 0.9)
		f.Cfg.Frontend.MaxLoopDuration = time.Duration(float64(timeout) * 0.9)
	}
}

func (f *Phlare) registerQuerierHandlers(svc querierv1connect.QuerierServiceHandler) {
	var (
		handlers = querier.NewHTTPHandlers(svc)
		wrap     = func(fn http.HandlerFunc) http.Handler {
			return util.AuthenticateUser(f.Cfg.MultitenancyEnabled).Wrap(fn)
		}
	)

	querierv1connect.RegisterQuerierServiceHandler(f.Server.HTTP, svc, f.auth)
	f.Server.HTTP.Handle("/pyroscope/render", wrap(handlers.Render))
	f.Server.HTTP.Handle("/pyroscope/label-values", wrap(handlers.LabelValues))
}

func (f *Phlare) initQuerier() (services.Service, error) {
	querierSvc, err := querier.New(f.Cfg.Querier, f.ring, nil, log.With(f.logger, "component", "querier"), f.auth)
	if err != nil {
		return nil, err
	}
	if !f.isModuleActive(QueryFrontend) {
		f.registerQuerierHandlers(querierSvc)
	}
	worker, err := worker.NewQuerierWorker(f.Cfg.Worker, querier.NewGRPCHandler(querierSvc), log.With(f.logger, "component", "querier-worker"), f.reg)
	if err != nil {
		return nil, err
	}

	sm, err := services.NewManager(querierSvc, worker)
	if err != nil {
		return nil, err
	}
	w := services.NewFailureWatcher()
	w.WatchManager(sm)

	return services.NewBasicService(func(ctx context.Context) error {
		err := sm.StartAsync(ctx)
		if err != nil {
			return err
		}
		return sm.AwaitHealthy(ctx)
	}, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case err := <-w.Chan():
			return err
		}
	}, func(failureCase error) error {
		sm.StopAsync()
		return sm.AwaitStopped(context.Background())
	}), nil
}

func (f *Phlare) getPusherClient() pushv1connect.PusherServiceClient {
	return f.pusherClient
}

func (f *Phlare) initGRPCGateway() (services.Service, error) {
	f.grpcGatewayMux = grpcgw.NewServeMux(
		grpcgw.WithMarshalerOption("application/json+pretty", &grpcgw.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				Indent:    "  ",
				Multiline: true, // Optional, implied by presence of "Indent".
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)
	return nil, nil
}

func (f *Phlare) initDistributor() (services.Service, error) {
	f.Cfg.Distributor.DistributorRing.ListenPort = f.Cfg.Server.HTTPListenPort
	d, err := distributor.New(f.Cfg.Distributor, f.ring, nil, f.Overrides, f.reg, log.With(f.logger, "component", "distributor"), f.auth)
	if err != nil {
		return nil, err
	}

	// initialise direct pusher, this overwrites the default HTTP client
	f.pusherClient = d
	pyroscopePath := "/pyroscope/ingest"
	f.Server.HTTP.Handle(pyroscopePath, util.AuthenticateUser(f.Cfg.MultitenancyEnabled).Wrap(pyroscope.NewPyroscopeIngestHandler(d, f.logger)))
	pushv1connect.RegisterPusherServiceHandler(f.Server.HTTP, d, f.auth)
	f.Server.HTTP.Path("/distributor/ring").Methods("GET", "POST").Handler(d)
	f.IndexPage.AddLinks(api.DefaultWeight, "Distributor", []api.IndexPageLink{
		{Desc: "Ring status", Path: "/distributor/ring"},
	})

	return d, nil
}

func (f *Phlare) initAgent() (services.Service, error) {
	a, err := agent.New(&f.Cfg.AgentConfig, log.With(f.logger, "component", "agent"), f.getPusherClient)
	if err != nil {
		return nil, err
	}
	f.agent = a

	// register endpoint at grpc gateway
	if err := agentv1.RegisterAgentServiceHandlerServer(context.Background(), f.grpcGatewayMux, a); err != nil {
		return nil, err
	}

	agentv1connect.RegisterAgentServiceHandler(f.Server.HTTP, a.ConnectHandler())
	return a, nil
}

func (f *Phlare) initMemberlistKV() (services.Service, error) {
	f.Cfg.MemberlistKV.MetricsRegisterer = f.reg
	f.Cfg.MemberlistKV.Codecs = []codec.Codec{
		ring.GetCodec(),
		usagestats.JSONCodec,
	}

	dnsProviderReg := prometheus.WrapRegistererWithPrefix(
		"phlare_",
		prometheus.WrapRegistererWith(
			prometheus.Labels{"name": "memberlist"},
			f.reg,
		),
	)
	dnsProvider := dns.NewProvider(f.logger, dnsProviderReg, dns.GolangResolverType)

	f.MemberlistKV = memberlist.NewKVInitService(&f.Cfg.MemberlistKV, f.logger, dnsProvider, f.reg)

	f.Cfg.Distributor.DistributorRing.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.Ingester.LifecyclerConfig.RingConfig.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.QueryScheduler.ServiceDiscovery.SchedulerRing.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.OverridesExporter.Ring.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV

	f.Cfg.Frontend.QuerySchedulerDiscovery = f.Cfg.QueryScheduler.ServiceDiscovery
	f.Cfg.Worker.QuerySchedulerDiscovery = f.Cfg.QueryScheduler.ServiceDiscovery

	f.Server.HTTP.Path("/memberlist").Handler(api.MemberlistStatusHandler("", f.MemberlistKV))
	f.IndexPage.AddLinks(api.MemberlistWeight, "Memberlist", []api.IndexPageLink{
		{Desc: "Status", Path: "/memberlist"},
	})
	return f.MemberlistKV, nil
}

func (f *Phlare) initRing() (_ services.Service, err error) {
	f.ring, err = ring.New(f.Cfg.Ingester.LifecyclerConfig.RingConfig, "ingester", "ring", log.With(f.logger, "component", "ring"), prometheus.WrapRegistererWithPrefix("phlare_", f.reg))
	if err != nil {
		return nil, err
	}
	f.Server.HTTP.Path("/ring").Methods("GET", "POST").Handler(f.ring)
	f.IndexPage.AddLinks(api.DefaultWeight, "Ingester", []api.IndexPageLink{
		{Desc: "Ring status", Path: "/ring"},
	})
	return f.ring, nil
}

func (f *Phlare) initStorage() (_ services.Service, err error) {
	objectStoreTypeStats.Set(f.Cfg.Storage.Bucket.Backend)
	if cfg := f.Cfg.Storage.Bucket; cfg.Backend != "filesystem" {
		b, err := objstoreclient.NewBucket(
			f.context(),
			cfg,
			"storage",
		)
		if err != nil {
			return nil, errors.Wrap(err, "unable to initialise bucket")
		}
		f.storageBucket = b
	}

	if f.Cfg.Target.String() != All && f.storageBucket == nil {
		return nil, errors.New("storage bucket configuration is required when running in microservices mode")
	}

	return nil, nil
}

// TODO: This should be passed to all other services and could also be used to signal shutdown
func (f *Phlare) context() context.Context {
	phlarectx := phlarecontext.WithLogger(context.Background(), f.logger)
	return phlarecontext.WithRegistry(phlarectx, f.reg)
}

func (f *Phlare) initIngester() (_ services.Service, err error) {
	f.Cfg.Ingester.LifecyclerConfig.ListenPort = f.Cfg.Server.HTTPListenPort

	svc, err := ingester.New(f.context(), f.Cfg.Ingester, f.Cfg.PhlareDB, f.storageBucket, f.Overrides)
	if err != nil {
		return nil, err
	}
	ingesterv1connect.RegisterIngesterServiceHandler(f.Server.HTTP, svc, f.auth)

	return svc, nil
}

func (f *Phlare) initServer() (services.Service, error) {
	f.reg.MustRegister(version.NewCollector("phlare"))
	f.reg.Unregister(collectors.NewGoCollector())
	// register collector with additional metrics
	f.reg.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
	))
	DisableSignalHandling(&f.Cfg.Server)
	f.Cfg.Server.Registerer = prometheus.WrapRegistererWithPrefix("phlare_", f.reg)
	// Not all default middleware works with http2 so we'll add then manually.
	// see https://github.com/grafana/phlare/issues/231
	f.Cfg.Server.DoNotAddDefaultHTTPMiddleware = true

	f.setupWorkerTimeout()
	if f.isModuleActive(QueryScheduler) {
		// to ensure that the query scheduler is always able to handle the request, we need to double the timeout
		f.Cfg.Server.HTTPServerReadTimeout = 2 * f.Cfg.Server.HTTPServerReadTimeout
		f.Cfg.Server.HTTPServerWriteTimeout = 2 * f.Cfg.Server.HTTPServerWriteTimeout
	}
	serv, err := server.New(f.Cfg.Server)
	if err != nil {
		return nil, err
	}

	f.Server = serv

	servicesToWaitFor := func() []services.Service {
		svs := []services.Service(nil)
		for m, s := range f.serviceMap {
			// Server should not wait for itself.
			if m != Server {
				svs = append(svs, s)
			}
		}
		return svs
	}

	httpMetric, err := util.NewHTTPMetricMiddleware(f.Server.HTTP, f.Cfg.Server.MetricsNamespace, f.Cfg.Server.Registerer)
	if err != nil {
		return nil, err
	}
	defaultHTTPMiddleware := []middleware.Interface{
		middleware.Tracer{
			RouteMatcher: f.Server.HTTP,
		},
		util.Log{
			Log:                   f.Server.Log,
			LogRequestAtInfoLevel: f.Cfg.Server.LogRequestAtInfoLevel,
		},
		httpMetric,
	}
	f.Server.HTTPServer.Handler = middleware.Merge(defaultHTTPMiddleware...).Wrap(f.Server.HTTP)

	s := NewServerService(f.Server, servicesToWaitFor, f.logger)
	// todo configure http2
	f.Server.HTTPServer.Handler = h2c.NewHandler(f.Server.HTTPServer.Handler, &http2.Server{})
	f.Server.HTTPServer.Handler = util.RecoveryHTTPMiddleware.Wrap(f.Server.HTTPServer.Handler)

	// expose openapiv2 definition
	openapiv2Handler, err := openapiv2.Handler()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize openapiv2 handler: %w", err)
	}
	f.Server.HTTP.Handle("/api/swagger.json", openapiv2Handler)

	// register grpc-gateway api
	f.Server.HTTP.NewRoute().PathPrefix("/api").Handler(f.grpcGatewayMux)
	// register fgprof
	f.Server.HTTP.Path("/debug/fgprof").Handler(fgprof.Handler())

	// register status service providing config and buildinfo at grpc gateway
	if err := statusv1.RegisterStatusServiceHandlerServer(context.Background(), f.grpcGatewayMux, f.statusService()); err != nil {
		return nil, err
	}

	// register static assets
	f.Server.HTTP.PathPrefix("/static/").Handler(http.FileServer(http.FS(api.StaticFiles)))

	// register ui
	uiAssets, err := public.Assets()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize the ui: %w", err)
	}
	f.Server.HTTP.PathPrefix("/ui/").Handler(http.StripPrefix("/ui/", http.FileServer(uiAssets)))

	// register index page
	f.IndexPage = api.NewIndexPageContent()
	f.Server.HTTP.Path("/").Handler(api.IndexHandler("", f.IndexPage))

	// Add index page links.
	f.IndexPage.AddLinks(api.BuildInfoWeight, "Build information", []api.IndexPageLink{
		{Desc: "Build information", Path: "/api/v1/status/buildinfo"},
	})
	f.IndexPage.AddLinks(api.ConfigWeight, "Current config", []api.IndexPageLink{
		{Desc: "Including the default values", Path: "/api/v1/status/config"},
		{Desc: "Only values that differ from the defaults", Path: "/api/v1/status/config/diff"},
		{Desc: "Default values", Path: "/api/v1/status/config/default"},
	})
	f.IndexPage.AddLinks(api.OpenAPIDefinitionWeight, "OpenAPI definition", []api.IndexPageLink{
		{Desc: "Swagger JSON", Path: "/api/swagger.json"},
	})

	return s, nil
}

func (f *Phlare) initUsageReport() (services.Service, error) {
	if !f.Cfg.Analytics.Enabled {
		return nil, nil
	}
	f.Cfg.Analytics.Leader = false
	// ingester is the only component that can be a leader
	if f.isModuleActive(Ingester) {
		f.Cfg.Analytics.Leader = true
	}

	usagestats.Target(f.Cfg.Target.String())

	b := f.storageBucket
	if f.storageBucket == nil {
		if err := os.MkdirAll(f.Cfg.PhlareDB.DataPath, 0o777); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", f.Cfg.PhlareDB.DataPath, err)
		}
		fs, err := filesystem.NewBucket(f.Cfg.PhlareDB.DataPath)
		if err != nil {
			return nil, err
		}
		b = fs
	}

	if b == nil {
		level.Warn(f.logger).Log("msg", "no storage bucket configured, usage report will not be sent")
		return nil, nil
	}

	ur, err := usagestats.NewReporter(f.Cfg.Analytics, f.Cfg.Ingester.LifecyclerConfig.RingConfig.KVStore, b, f.logger, f.reg)
	if err != nil {
		level.Info(f.logger).Log("msg", "failed to initialize usage report", "err", err)
		return nil, nil
	}
	f.usageReport = ur
	return ur, nil
}

type statusService struct {
	statusv1.UnimplementedStatusServiceServer
	defaultConfig *Config
	actualConfig  *Config
}

func (s *statusService) GetBuildInfo(ctx context.Context, req *statusv1.GetBuildInfoRequest) (*statusv1.GetBuildInfoResponse, error) {
	version := build.GetVersion()
	return &statusv1.GetBuildInfoResponse{
		Status: "success",
		Data: &statusv1.GetBuildInfoData{
			Version:   version.Version,
			Revision:  build.Revision,
			Branch:    version.Branch,
			GoVersion: version.GoVersion,
		},
	}, nil
}

const (
	// There is not standardised and generally used content-type for YAML,
	// text/plain ensures the YAML is displayed in the browser instead of
	// offered as a download
	yamlContentType = "text/plain; charset=utf-8"
)

func (s *statusService) GetConfig(ctx context.Context, req *statusv1.GetConfigRequest) (*httpbody.HttpBody, error) {
	body, err := yaml.Marshal(s.actualConfig)
	if err != nil {
		return nil, err
	}

	return &httpbody.HttpBody{
		ContentType: yamlContentType,
		Data:        body,
	}, nil
}

func (s *statusService) GetDefaultConfig(ctx context.Context, req *statusv1.GetConfigRequest) (*httpbody.HttpBody, error) {
	body, err := yaml.Marshal(s.defaultConfig)
	if err != nil {
		return nil, err
	}

	return &httpbody.HttpBody{
		ContentType: yamlContentType,
		Data:        body,
	}, nil
}

func (s *statusService) GetDiffConfig(ctx context.Context, req *statusv1.GetConfigRequest) (*httpbody.HttpBody, error) {
	aBody, err := yaml.Marshal(s.actualConfig)
	if err != nil {
		return nil, err
	}
	aCfg := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(aBody, &aCfg); err != nil {
		return nil, err
	}

	dBody, err := yaml.Marshal(s.defaultConfig)
	if err != nil {
		return nil, err
	}
	dCfg := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(dBody, &dCfg); err != nil {
		return nil, err
	}

	diff, err := util.DiffConfig(dCfg, aCfg)
	if err != nil {
		return nil, err
	}

	body, err := yaml.Marshal(diff)
	if err != nil {
		return nil, err
	}

	return &httpbody.HttpBody{
		ContentType: yamlContentType,
		Data:        body,
	}, nil
}

func (f *Phlare) statusService() statusv1.StatusServiceServer {
	return &statusService{
		actualConfig:  &f.Cfg,
		defaultConfig: newDefaultConfig(),
	}
}

func (f *Phlare) isModuleActive(m string) bool {
	for _, target := range f.Cfg.Target {
		if target == m {
			return true
		}
		if f.recursiveIsModuleActive(target, m) {
			return true
		}
	}
	return false
}

func (f *Phlare) recursiveIsModuleActive(target, m string) bool {
	if targetDeps, ok := f.deps[target]; ok {
		for _, dep := range targetDeps {
			if dep == m {
				return true
			}
			if f.recursiveIsModuleActive(dep, m) {
				return true
			}
		}
	}
	return false
}

// NewServerService constructs service from Server component.
// servicesToWaitFor is called when server is stopping, and should return all
// services that need to terminate before server actually stops.
// N.B.: this function is NOT Cortex specific, please let's keep it that way.
// Passed server should not react on signals. Early return from Run function is considered to be an error.
func NewServerService(serv *server.Server, servicesToWaitFor func() []services.Service, log log.Logger) services.Service {
	serverDone := make(chan error, 1)

	runFn := func(ctx context.Context) error {
		go func() {
			defer close(serverDone)
			serverDone <- serv.Run()
		}()

		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			return fmt.Errorf("server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// wait until all modules are done, and then shutdown server.
		for _, s := range servicesToWaitFor() {
			_ = s.AwaitTerminated(context.Background())
		}

		// shutdown HTTP and gRPC servers (this also unblocks Run)
		serv.Shutdown()

		// if not closed yet, wait until server stops.
		<-serverDone
		level.Info(log).Log("msg", "server stopped")
		return nil
	}

	return services.NewBasicService(nil, runFn, stoppingFn)
}

// DisableSignalHandling puts a dummy signal handler
func DisableSignalHandling(config *server.Config) {
	config.SignalHandler = make(ignoreSignalHandler)
}

type ignoreSignalHandler chan struct{}

func (dh ignoreSignalHandler) Loop() {
	<-dh
}

func (dh ignoreSignalHandler) Stop() {
	close(dh)
}
