package phlare

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/dns"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/runtimeconfig"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/services"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	objstoretracing "github.com/thanos-io/objstore/tracing/opentracing"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"

	statusv1 "github.com/grafana/pyroscope/api/gen/proto/go/status/v1"
	"github.com/grafana/pyroscope/pkg/adhocprofiles"
	apiversion "github.com/grafana/pyroscope/pkg/api/version"
	"github.com/grafana/pyroscope/pkg/compactor"
	"github.com/grafana/pyroscope/pkg/distributor"
	"github.com/grafana/pyroscope/pkg/embedded/grafana"
	"github.com/grafana/pyroscope/pkg/frontend"
	readpath "github.com/grafana/pyroscope/pkg/frontend/read_path"
	queryfrontend "github.com/grafana/pyroscope/pkg/frontend/read_path/query_frontend"
	"github.com/grafana/pyroscope/pkg/ingester"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/operations"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/querier"
	"github.com/grafana/pyroscope/pkg/querier/vcs"
	"github.com/grafana/pyroscope/pkg/querier/worker"
	"github.com/grafana/pyroscope/pkg/scheduler"
	"github.com/grafana/pyroscope/pkg/settings"
	"github.com/grafana/pyroscope/pkg/storegateway"
	"github.com/grafana/pyroscope/pkg/usagestats"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/build"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/validation"
	"github.com/grafana/pyroscope/pkg/validation/exporter"
)

// The various modules that make up Pyroscope.
const (
	All               string = "all"
	API               string = "api"
	Version           string = "version"
	Distributor       string = "distributor"
	Server            string = "server"
	IngesterRing      string = "ring"
	Ingester          string = "ingester"
	MemberlistKV      string = "memberlist-kv"
	Querier           string = "querier"
	StoreGateway      string = "store-gateway"
	GRPCGateway       string = "grpc-gateway"
	Storage           string = "storage"
	UsageReport       string = "usage-stats"
	QueryFrontend     string = "query-frontend"
	QueryScheduler    string = "query-scheduler"
	RuntimeConfig     string = "runtime-config"
	Overrides         string = "overrides"
	OverridesExporter string = "overrides-exporter"
	Compactor         string = "compactor"
	Admin             string = "admin"
	TenantSettings    string = "tenant-settings"
	AdHocProfiles     string = "ad-hoc-profiles"
	EmbeddedGrafana   string = "embedded-grafana"

	// Experimental modules

	Metastore           string = "metastore"
	MetastoreClient     string = "metastore-client"
	SegmentWriter       string = "segment-writer"
	SegmentWriterRing   string = "segment-writer-ring"
	SegmentWriterClient string = "segment-writer-client"
	QueryBackend        string = "query-backend"
	QueryBackendClient  string = "query-backend-client"
	CompactionWorker    string = "compaction-worker"
	PlacementAgent      string = "placement-agent"
	PlacementManager    string = "placement-manager"
	HealthServer        string = "health-server"
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

	frontendSvc, err := frontend.NewFrontend(f.Cfg.Frontend, f.Overrides, log.With(f.logger, "component", "frontend"), f.reg)
	if err != nil {
		return nil, err
	}
	f.frontend = frontendSvc
	f.API.RegisterFrontendForQuerierHandler(frontendSvc)
	if !f.Cfg.v2Experiment {
		f.API.RegisterQuerierServiceHandler(frontendSvc)
		f.API.RegisterPyroscopeHandlers(frontendSvc)
		f.API.RegisterVCSServiceHandler(frontendSvc)
	} else {
		f.initReadPathRouter()
	}

	return frontendSvc, nil
}

func (f *Phlare) initReadPathRouter() {
	vcsService := vcs.New(
		log.With(f.logger, "component", "vcs-service"),
		f.reg,
	)

	newFrontend := queryfrontend.NewQueryFrontend(
		log.With(f.logger, "component", "query-frontend"),
		f.Overrides,
		f.metastoreClient,
		f.queryBackendClient,
	)

	router := readpath.NewRouter(
		log.With(f.logger, "component", "read-path-router"),
		f.Overrides,
		f.frontend,
		newFrontend,
	)

	f.API.RegisterQuerierServiceHandler(router)
	f.API.RegisterPyroscopeHandlers(router)
	f.API.RegisterVCSServiceHandler(vcsService)
}

func (f *Phlare) initRuntimeConfig() (services.Service, error) {
	if len(f.Cfg.RuntimeConfig.LoadPath) == 0 {
		// no need to initialize module if load path is empty
		return nil, nil
	}

	f.Cfg.RuntimeConfig.Loader = func(r io.Reader) (interface{}, error) {
		return validation.LoadRuntimeConfig(r)
	}

	// make sure to set default limits before we start loading configuration into memory
	validation.SetDefaultLimitsForYAMLUnmarshalling(f.Cfg.LimitsConfig)

	serv, err := runtimeconfig.New(f.Cfg.RuntimeConfig, "pyroscope", prometheus.WrapRegistererWithPrefix("pyroscope_", f.reg), log.With(f.logger, "component", "runtime-config"))
	if err == nil {
		// TenantLimits just delegates to RuntimeConfig and doesn't have any state or need to do
		// anything in the start/stopping phase. Thus we can create it as part of runtime config
		// setup without any service instance of its own.
		f.TenantLimits = newTenantLimits(serv)
	}

	f.RuntimeConfig = serv
	f.API.RegisterRuntimeConfig(runtimeConfigHandler(f.RuntimeConfig, f.Cfg.LimitsConfig), validation.TenantLimitsHandler(f.Cfg.LimitsConfig, f.TenantLimits))

	return serv, err
}

func (f *Phlare) initTenantSettings() (services.Service, error) {
	var store settings.Store
	var err error

	switch {
	case f.storageBucket != nil:
		store, err = settings.NewBucketStore(f.storageBucket)
	default:
		store, err = settings.NewMemoryStore()
		level.Warn(f.logger).Log("msg", "using in-memory settings store, changes will be lost after shutdown")
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to init settings store")
	}

	settings, err := settings.New(store, log.With(f.logger, "component", TenantSettings))
	if err != nil {
		return nil, errors.Wrap(err, "failed to init settings service")
	}

	f.API.RegisterTenantSettings(settings)
	return settings, nil
}

func (f *Phlare) initAdHocProfiles() (services.Service, error) {
	if f.storageBucket == nil {
		level.Warn(f.logger).Log("msg", "no storage bucket configured, ad hoc profiles will not be loaded")
		return nil, nil
	}

	a := adhocprofiles.NewAdHocProfiles(f.storageBucket, f.logger, f.Overrides)
	f.API.RegisterAdHocProfiles(a)
	return a, nil
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

	f.API.RegisterOverridesExporter(overridesExporter)

	return overridesExporter, nil
}

func (f *Phlare) initQueryScheduler() (services.Service, error) {
	f.Cfg.QueryScheduler.ServiceDiscovery.SchedulerRing.ListenPort = f.Cfg.Server.HTTPListenPort

	s, err := scheduler.NewScheduler(f.Cfg.QueryScheduler, f.Overrides, log.With(f.logger, "component", "scheduler"), f.reg)
	if err != nil {
		return nil, errors.Wrap(err, "query-scheduler init")
	}

	f.API.RegisterQueryScheduler(s)

	return s, nil
}

func (f *Phlare) initCompactor() (serv services.Service, err error) {
	f.Cfg.Compactor.ShardingRing.Common.ListenPort = f.Cfg.Server.HTTPListenPort

	if f.storageBucket == nil {
		return nil, nil
	}

	f.Compactor, err = compactor.NewMultitenantCompactor(f.Cfg.Compactor, f.storageBucket, f.Overrides, log.With(f.logger, "component", "compactor"), f.reg)
	if err != nil {
		return
	}

	// Expose HTTP endpoints.
	f.API.RegisterCompactor(f.Compactor)
	return f.Compactor, nil
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

func (f *Phlare) initQuerier() (services.Service, error) {
	newQuerierParams := &querier.NewQuerierParams{
		Cfg:             f.Cfg.Querier,
		StoreGatewayCfg: f.Cfg.StoreGateway,
		Overrides:       f.Overrides,
		CfgProvider:     f.Overrides,
		StorageBucket:   f.storageBucket,
		IngestersRing:   f.ingesterRing,
		Reg:             f.reg,
		Logger:          log.With(f.logger, "component", "querier"),
		ClientOptions:   []connect.ClientOption{f.auth},
	}
	querierSvc, err := querier.New(newQuerierParams)
	if err != nil {
		return nil, err
	}

	if !f.isModuleActive(QueryFrontend) {
		f.API.RegisterPyroscopeHandlers(querierSvc)
		f.API.RegisterQuerierServiceHandler(querierSvc)
		f.API.RegisterVCSServiceHandler(querierSvc)
	}

	qWorker, err := worker.NewQuerierWorker(
		f.Cfg.Worker,
		querier.NewGRPCHandler(querierSvc, f.Cfg.SelfProfiling.UseK6Middleware),
		log.With(f.logger, "component", "querier-worker"),
		f.reg,
	)
	if err != nil {
		return nil, err
	}

	sm, err := services.NewManager(querierSvc, qWorker)
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
	logger := log.With(f.logger, "component", "distributor")
	d, err := distributor.New(f.Cfg.Distributor, f.ingesterRing, nil, f.Overrides, f.reg, logger, f.segmentWriterClient, f.auth)
	if err != nil {
		return nil, err
	}
	f.API.RegisterDistributor(d)
	return d, nil
}

func (f *Phlare) initMemberlistKV() (services.Service, error) {
	f.Cfg.MemberlistKV.Codecs = []codec.Codec{
		ring.GetCodec(),
		usagestats.JSONCodec,
		apiversion.GetCodec(),
	}

	dnsProviderReg := prometheus.WrapRegistererWithPrefix(
		"pyroscope_",
		prometheus.WrapRegistererWith(
			prometheus.Labels{"name": "memberlist"},
			f.reg,
		),
	)
	dnsProvider := dns.NewProvider(f.logger, dnsProviderReg, dns.GolangResolverType)

	f.MemberlistKV = memberlist.NewKVInitService(&f.Cfg.MemberlistKV, f.logger, dnsProvider, f.reg)

	f.Cfg.Distributor.DistributorRing.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.Ingester.LifecyclerConfig.RingConfig.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.SegmentWriter.LifecyclerConfig.RingConfig.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.QueryScheduler.ServiceDiscovery.SchedulerRing.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.OverridesExporter.Ring.Ring.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.StoreGateway.ShardingRing.Ring.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.Compactor.ShardingRing.Common.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV
	f.Cfg.Frontend.QuerySchedulerDiscovery = f.Cfg.QueryScheduler.ServiceDiscovery
	f.Cfg.Worker.QuerySchedulerDiscovery = f.Cfg.QueryScheduler.ServiceDiscovery

	f.API.RegisterMemberlistKV("", f.MemberlistKV)

	return f.MemberlistKV, nil
}

func (f *Phlare) initIngesterRing() (_ services.Service, err error) {
	f.ingesterRing, err = ring.New(f.Cfg.Ingester.LifecyclerConfig.RingConfig, "ingester", "ring", log.With(f.logger, "component", "ring"), prometheus.WrapRegistererWithPrefix("pyroscope_", f.reg))
	if err != nil {
		return nil, err
	}
	f.API.RegisterIngesterRing(f.ingesterRing)
	return f.ingesterRing, nil
}

func (f *Phlare) initStorage() (_ services.Service, err error) {
	objectStoreTypeStats.Set(f.Cfg.Storage.Bucket.Backend)
	if cfg := f.Cfg.Storage.Bucket; cfg.Backend != objstoreclient.None {
		if cfg.Backend == objstoreclient.Filesystem {
			level.Warn(f.logger).Log("msg", "when running with storage.backend 'filesystem' it is important that all replicas/components share the same filesystem")
		}
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

	if !slices.Contains(f.Cfg.Target, All) && f.storageBucket == nil {
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

	svc, err := ingester.New(f.context(), f.Cfg.Ingester, f.Cfg.PhlareDB, f.storageBucket, f.Overrides, f.Cfg.Querier.QueryStoreAfter)
	if err != nil {
		return nil, err
	}

	f.API.RegisterIngester(svc)
	f.ingester = svc

	return svc, nil
}

func (f *Phlare) initStoreGateway() (serv services.Service, err error) {
	f.Cfg.StoreGateway.ShardingRing.Ring.ListenPort = f.Cfg.Server.HTTPListenPort
	if f.storageBucket == nil {
		return nil, nil
	}

	svc, err := storegateway.NewStoreGateway(f.Cfg.StoreGateway, f.storageBucket, f.Overrides, f.logger, f.reg)
	if err != nil {
		return nil, err
	}
	f.API.RegisterStoreGateway(svc)
	return svc, nil
}

var objstoreTracerMiddleware = middleware.Func(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if tracer := opentracing.GlobalTracer(); tracer != nil {
			ctx = objstoretracing.ContextWithTracer(ctx, opentracing.GlobalTracer())
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
})

func (f *Phlare) initServer() (services.Service, error) {
	f.reg.MustRegister(version.NewCollector("pyroscope"))
	f.reg.Unregister(collectors.NewGoCollector())
	// register collector with additional metrics
	f.reg.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
	))
	DisableSignalHandling(&f.Cfg.Server)
	f.Cfg.Server.Registerer = prometheus.WrapRegistererWithPrefix("pyroscope_", f.reg)
	// Not all default middleware works with http2 so we'll add then manually.
	// see https://github.com/grafana/pyroscope/issues/231
	f.Cfg.Server.DoNotAddDefaultHTTPMiddleware = true
	f.Cfg.Server.ExcludeRequestInLog = true // gRPC-specific.
	f.Cfg.Server.GRPCMiddleware = append(f.Cfg.Server.GRPCMiddleware, util.RecoveryInterceptorGRPC)

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
	if f.Cfg.v2Experiment {
		grpc_health_v1.RegisterHealthServer(f.Server.GRPC, f.healthServer)
	}

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
		objstoreTracerMiddleware,
		httputil.K6Middleware(),
	}
	if f.Cfg.SelfProfiling.UseK6Middleware {
		defaultHTTPMiddleware = append(defaultHTTPMiddleware, httputil.K6Middleware())
	}

	f.Server.HTTPServer.Handler = middleware.Merge(defaultHTTPMiddleware...).Wrap(f.Server.HTTP)

	s := NewServerService(f.Server, servicesToWaitFor, f.logger)
	// todo configure http2
	f.Server.HTTPServer.Handler = h2c.NewHandler(f.Server.HTTPServer.Handler, &http2.Server{})
	f.Server.HTTPServer.Handler = util.RecoveryHTTPMiddleware.Wrap(f.Server.HTTPServer.Handler)

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

func (f *Phlare) initAdmin() (services.Service, error) {
	if f.storageBucket == nil {
		level.Warn(f.logger).Log("msg", "no storage bucket configured, the admin component will not be loaded")
		return nil, nil
	}

	a, err := operations.NewAdmin(f.storageBucket, f.logger, f.Cfg.PhlareDB.MaxBlockDuration)
	if err != nil {
		level.Info(f.logger).Log("msg", "failed to initialize admin", "err", err)
		return nil, nil
	}
	f.admin = a
	f.API.RegisterAdmin(a)
	return a, nil
}

func (f *Phlare) initEmbeddedGrafana() (services.Service, error) {
	return grafana.New(f.Cfg.EmbeddedGrafana, f.logger)
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
				return fmt.Errorf("server stopped unexpectedly: %w", err)
			}
			return nil
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
