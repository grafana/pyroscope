package phlare

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/netutil"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"

	compactionworker "github.com/grafana/pyroscope/pkg/experiment/compactor"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	segmentwriter "github.com/grafana/pyroscope/pkg/experiment/ingester"
	segmentwriterclient "github.com/grafana/pyroscope/pkg/experiment/ingester/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
	metastoreadmin "github.com/grafana/pyroscope/pkg/experiment/metastore/admin"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/experiment/metrics"
	querybackend "github.com/grafana/pyroscope/pkg/experiment/query_backend"
	querybackendclient "github.com/grafana/pyroscope/pkg/experiment/query_backend/client"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
	"github.com/grafana/pyroscope/pkg/frontend"
	readpath "github.com/grafana/pyroscope/pkg/frontend/read_path"
	queryfrontend "github.com/grafana/pyroscope/pkg/frontend/read_path/query_frontend"
	"github.com/grafana/pyroscope/pkg/frontend/vcs"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	recordingrulesclient "github.com/grafana/pyroscope/pkg/settings/recording/client"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/health"
)

func (f *Phlare) initQueryFrontend() (services.Service, error) {
	var err error
	if f.Cfg.Frontend.Addr, err = f.getFrontendAddress(); err != nil {
		return nil, fmt.Errorf("failed to get frontend address: %w", err)
	}
	if f.Cfg.Frontend.Port == 0 {
		f.Cfg.Frontend.Port = f.Cfg.Server.HTTPListenPort
	}
	if !f.Cfg.v2Experiment {
		return f.initQueryFrontendV1()
	}
	// If the new read path is enabled globally by default,
	// the old query frontend is not used. Tenant-specific overrides
	// are ignored — all tenants use the new read path.
	//
	// If the old read path is still in use, we configure the router
	// to use both the old and new query frontends.
	c := f.Overrides.ReadPathOverrides(tenant.DefaultTenantID)
	switch {
	case !c.EnableQueryBackend:
		return f.initQueryFrontendV1()
	case c.EnableQueryBackend && c.EnableQueryBackendFrom.IsZero():
		return f.initQueryFrontendV2()
	case c.EnableQueryBackend && !c.EnableQueryBackendFrom.IsZero():
		return f.initQueryFrontendV12()
	default:
		return nil, fmt.Errorf("invalid query backend configuration: %v", c)
	}
}

func (f *Phlare) initQueryFrontendV1() (services.Service, error) {
	var err error
	f.frontend, err = frontend.NewFrontend(f.Cfg.Frontend, f.Overrides, log.With(f.logger, "component", "frontend"), f.reg)
	if err != nil {
		return nil, err
	}
	f.API.RegisterFrontendForQuerierHandler(f.frontend)
	f.API.RegisterQuerierServiceHandler(f.frontend)
	f.API.RegisterPyroscopeHandlers(f.frontend)
	f.API.RegisterVCSServiceHandler(f.frontend)
	return f.frontend, nil
}

func (f *Phlare) initQueryFrontendV2() (services.Service, error) {
	queryFrontend := queryfrontend.NewQueryFrontend(
		log.With(f.logger, "component", "query-frontend"),
		f.Overrides,
		f.metastoreClient,
		f.metastoreClient,
		f.queryBackendClient,
		f.symbolizer,
	)

	vcsService := vcs.New(
		log.With(f.logger, "component", "vcs-service"),
		f.reg,
	)

	f.API.RegisterQuerierServiceHandler(queryFrontend)
	f.API.RegisterPyroscopeHandlers(queryFrontend)
	f.API.RegisterVCSServiceHandler(vcsService)

	// New query frontend does not have any state.
	// For simplicity, we return a no-op service.
	svc := services.NewIdleService(
		func(context.Context) error { return nil },
		func(error) error { return nil },
	)

	return svc, nil
}

func (f *Phlare) initQueryFrontendV12() (services.Service, error) {
	var err error
	f.frontend, err = frontend.NewFrontend(f.Cfg.Frontend, f.Overrides, log.With(f.logger, "component", "frontend"), f.reg)
	if err != nil {
		return nil, err
	}

	newFrontend := queryfrontend.NewQueryFrontend(
		log.With(f.logger, "component", "query-frontend"),
		f.Overrides,
		f.metastoreClient,
		f.metastoreClient,
		f.queryBackendClient,
		f.symbolizer,
	)

	handler := readpath.NewRouter(
		log.With(f.logger, "component", "read-path-router"),
		f.Overrides,
		f.frontend,
		newFrontend,
	)

	vcsService := vcs.New(
		log.With(f.logger, "component", "vcs-service"),
		f.reg,
	)

	f.API.RegisterFrontendForQuerierHandler(f.frontend)
	f.API.RegisterQuerierServiceHandler(handler)
	f.API.RegisterPyroscopeHandlers(handler)
	f.API.RegisterVCSServiceHandler(vcsService)

	return f.frontend, nil
}

func (f *Phlare) getFrontendAddress() (addr string, err error) {
	addr = f.Cfg.Frontend.Addr
	if f.Cfg.Frontend.AddrOld != "" {
		addr = f.Cfg.Frontend.AddrOld
	}
	if addr != "" {
		return addr, nil
	}
	return netutil.GetFirstAddressOf(f.Cfg.Frontend.InfNames, f.logger, f.Cfg.Frontend.EnableIPv6)
}

func (f *Phlare) initSegmentWriterRing() (_ services.Service, err error) {
	if err = f.Cfg.SegmentWriter.Validate(); err != nil {
		return nil, err
	}
	logger := log.With(f.logger, "component", "segment-writer-ring")
	reg := prometheus.WrapRegistererWithPrefix("pyroscope_", f.reg)
	f.segmentWriterRing, err = ring.New(
		f.Cfg.SegmentWriter.LifecyclerConfig.RingConfig,
		segmentwriter.RingName,
		segmentwriter.RingKey,
		logger, reg,
	)
	if err != nil {
		return nil, err
	}
	f.API.RegisterSegmentWriterRing(f.segmentWriterRing)
	return f.segmentWriterRing, nil
}

func (f *Phlare) initSegmentWriter() (services.Service, error) {
	f.Cfg.SegmentWriter.LifecyclerConfig.ListenPort = f.Cfg.Server.GRPCListenPort
	if err := f.Cfg.SegmentWriter.Validate(); err != nil {
		return nil, err
	}

	logger := log.With(f.logger, "component", "segment-writer")
	healthService := health.NewGRPCHealthService(f.healthServer, logger, "pyroscope.segment-writer")
	segmentWriter, err := segmentwriter.New(
		f.reg,
		logger,
		f.Cfg.SegmentWriter,
		f.Overrides,
		healthService,
		f.storageBucket,
		f.metastoreClient,
	)
	if err != nil {
		return nil, err
	}

	f.segmentWriter = segmentWriter
	f.API.RegisterSegmentWriter(segmentWriter)
	return f.segmentWriter, nil
}

func (f *Phlare) initSegmentWriterClient() (_ services.Service, err error) {
	f.Cfg.SegmentWriter.GRPCClientConfig.Middleware = f.grpcClientInterceptors()
	// Validation of the config is not required since
	// it's already validated in initSegmentWriterRing.
	logger := log.With(f.logger, "component", "segment-writer-client")
	placement := f.placementAgent.Placement()
	client, err := segmentwriterclient.NewSegmentWriterClient(
		f.Cfg.SegmentWriter.GRPCClientConfig,
		logger, f.reg,
		f.segmentWriterRing,
		placement,
	)
	if err != nil {
		return nil, err
	}
	f.segmentWriterClient = client
	return client.Service(), nil
}

func (f *Phlare) initCompactionWorker() (svc services.Service, err error) {
	logger := log.With(f.logger, "component", "compaction-worker")
	registerer := prometheus.WrapRegistererWithPrefix("pyroscope_compaction_worker_", f.reg)

	var ruler metrics.Ruler
	var exporter metrics.Exporter
	if f.Cfg.CompactionWorker.MetricsExporter.Enabled {
		if f.recordingRulesClient != nil {
			ruler, err = metrics.NewCachedRemoteRuler(f.recordingRulesClient, f.logger)
			if err != nil {
				return nil, err
			}
		} else {
			ruler = metrics.NewStaticRulerFromOverrides(f.Overrides)
		}

		exporter, err = metrics.NewExporter(f.Cfg.CompactionWorker.MetricsExporter.RemoteWriteAddress, f.logger, f.reg)
		if err != nil {
			return nil, err
		}
	}

	w, err := compactionworker.New(
		logger,
		f.Cfg.CompactionWorker,
		f.metastoreClient,
		f.storageBucket,
		registerer,
		ruler,
		exporter,
	)
	if err != nil {
		return nil, err
	}
	f.compactionWorker = w
	return w.Service(), nil
}

func (f *Phlare) initMetastore() (services.Service, error) {
	if err := f.Cfg.Metastore.Validate(); err != nil {
		return nil, err
	}

	logger := log.With(f.logger, "component", "metastore")
	healthService := health.NewGRPCHealthService(f.healthServer, logger, "pyroscope.metastore")
	registerer := prometheus.WrapRegistererWithPrefix("pyroscope_metastore_", f.reg)
	m, err := metastore.New(
		f.Cfg.Metastore,
		logger,
		registerer,
		healthService,
		f.metastoreClient,
		f.storageBucket,
		f.placementManager,
	)
	if err != nil {
		return nil, err
	}

	m.Register(f.Server.GRPC)
	f.metastore = m
	return m.Service(), nil
}

func (f *Phlare) initMetastoreClient() (services.Service, error) {
	if err := f.Cfg.Metastore.Validate(); err != nil {
		return nil, err
	}

	disc, err := discovery.NewDiscovery(f.logger, f.Cfg.Metastore.Address, f.reg)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery: %w %s", err, f.Cfg.Metastore.Address)
	}

	f.Cfg.Metastore.GRPCClientConfig.Middleware = f.grpcClientInterceptors()
	f.metastoreClient = metastoreclient.New(
		f.logger,
		f.Cfg.Metastore.GRPCClientConfig,
		disc,
	)
	return f.metastoreClient.Service(), nil
}

func (f *Phlare) initMetastoreAdmin() (services.Service, error) {
	level.Info(f.logger).Log("msg", "initializing metastore admin")
	if err := f.Cfg.Metastore.Validate(); err != nil {
		return nil, err
	}

	var err error
	f.metastoreAdmin, err = metastoreadmin.New(f.metastoreClient, f.logger, f.Cfg.Metastore.Address, f.metastoreClient)
	if err != nil {
		return nil, err
	}
	level.Info(f.logger).Log("msg", "registering metastore admin routes")
	f.API.RegisterMetastoreAdmin(f.metastoreAdmin)
	return f.metastoreAdmin.Service(), nil
}

func (f *Phlare) initQueryBackend() (services.Service, error) {
	if err := f.Cfg.QueryBackend.Validate(); err != nil {
		return nil, err
	}
	logger := log.With(f.logger, "component", "query-backend")
	b, err := querybackend.New(
		f.Cfg.QueryBackend,
		logger,
		f.reg,
		f.queryBackendClient,
		querybackend.NewBlockReader(f.logger, f.storageBucket),
	)
	if err != nil {
		return nil, err
	}
	f.API.RegisterQueryBackend(b)
	return b.Service(), nil
}

func (f *Phlare) initQueryBackendClient() (services.Service, error) {
	if err := f.Cfg.QueryBackend.Validate(); err != nil {
		return nil, err
	}
	f.Cfg.QueryBackend.GRPCClientConfig.Middleware = f.grpcClientInterceptors()
	c, err := querybackendclient.New(
		f.Cfg.QueryBackend.Address,
		f.Cfg.QueryBackend.GRPCClientConfig,
	)
	if err != nil {
		return nil, err
	}
	f.queryBackendClient = c
	return c.Service(), nil
}

func (f *Phlare) initRecordingRulesClient() (services.Service, error) {
	if err := f.Cfg.CompactionWorker.MetricsExporter.Validate(); err != nil {
		return nil, err
	}
	if !f.Cfg.CompactionWorker.MetricsExporter.Enabled ||
		f.Cfg.CompactionWorker.MetricsExporter.RulesSource.ClientAddress == "" {
		return nil, nil
	}
	c, err := recordingrulesclient.NewClient(f.Cfg.CompactionWorker.MetricsExporter.RulesSource.ClientAddress, f.logger, f.auth)
	if err != nil {
		return nil, err
	}
	f.recordingRulesClient = c
	return c.Service(), nil
}

func (f *Phlare) initSymbolizer() (services.Service, error) {
	if !f.Cfg.Symbolizer.Enabled {
		level.Info(f.logger).Log("msg", "Symbolizer is disabled")
		return nil, nil
	}

	prefixedBucket := phlareobj.NewPrefixedBucket(f.storageBucket, "symbolizer")

	sym, err := symbolizer.New(
		f.logger,
		f.Cfg.Symbolizer,
		f.reg,
		prefixedBucket,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create symbolizer: %w", err)
	}

	f.symbolizer = sym

	return nil, nil
}

func (f *Phlare) initPlacementAgent() (services.Service, error) {
	f.placementAgent = adaptiveplacement.NewAgent(
		f.logger,
		f.reg,
		f.Cfg.AdaptivePlacement,
		f.Overrides,
		f.adaptivePlacementStore(),
	)
	return f.placementAgent.Service(), nil
}

func (f *Phlare) initPlacementManager() (services.Service, error) {
	f.placementManager = adaptiveplacement.NewManager(
		f.logger,
		f.reg,
		f.Cfg.AdaptivePlacement,
		f.Overrides,
		f.adaptivePlacementStore(),
	)
	return f.placementManager.Service(), nil
}

func (f *Phlare) adaptivePlacementStore() adaptiveplacement.Store {
	if slices.Contains(f.Cfg.Target, All) {
		// Disables sharding in all-in-one scenario.
		return adaptiveplacement.NewEmptyStore()
	}
	return adaptiveplacement.NewStore(f.storageBucket)
}

func (f *Phlare) initHealthServer() (services.Service, error) {
	f.healthServer = grpchealth.NewServer()
	return nil, nil
}

func (f *Phlare) grpcClientInterceptors() []grpc.UnaryClientInterceptor {
	requestDuration := util.RegisterOrGet(f.reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:                       "pyroscope",
		Subsystem:                       "grpc_client",
		Name:                            "request_duration_seconds",
		Help:                            "Time (in seconds) spent waiting for gRPC response.",
		Buckets:                         prometheus.DefBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  50,
		NativeHistogramMinResetDuration: time.Hour,
	}, []string{"method", "status_code"}))

	return []grpc.UnaryClientInterceptor{
		middleware.UnaryClientInstrumentInterceptor(requestDuration, middleware.ReportGRPCStatusOption),
		otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer()),
	}
}
