package phlare

import (
	"fmt"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"
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
	recordingrulesclient "github.com/grafana/pyroscope/pkg/settings/recording/client"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/health"
)

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
		} else {
			ruler, err = metrics.NewStaticRulerFromEnvVars(f.logger)
		}
		if err != nil {
			return nil, err
		}

		exporter, err = metrics.NewStaticExporterFromEnvVars(f.logger, f.reg)
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
