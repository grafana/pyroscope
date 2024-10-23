package phlare

import (
	"fmt"
	"slices"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	compactionworker "github.com/grafana/pyroscope/pkg/experiment/compactor"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	segmentwriter "github.com/grafana/pyroscope/pkg/experiment/ingester"
	segmentwriterclient "github.com/grafana/pyroscope/pkg/experiment/ingester/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	querybackend "github.com/grafana/pyroscope/pkg/experiment/query_backend"
	querybackendclient "github.com/grafana/pyroscope/pkg/experiment/query_backend/client"
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
	segmentWriter, err := segmentwriter.New(
		f.reg,
		f.logger,
		f.Cfg.SegmentWriter,
		f.Overrides,
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
	if err = f.Cfg.CompactionWorker.Validate(); err != nil {
		return nil, err
	}
	logger := log.With(f.logger, "component", "compaction-worker")
	f.compactionWorker, err = compactionworker.New(
		f.Cfg.CompactionWorker,
		logger,
		f.metastoreClient,
		f.storageBucket,
		f.reg,
	)
	if err != nil {
		return nil, err
	}
	return f.compactionWorker, nil
}

func (f *Phlare) initMetastore() (services.Service, error) {
	if err := f.Cfg.Metastore.Validate(); err != nil {
		return nil, err
	}
	logger := log.With(f.logger, "component", "metastore")
	m, err := metastore.New(
		f.Cfg.Metastore,
		logger,
		f.reg,
		f.metastoreClient,
		f.storageBucket,
		f.placementManager,
	)
	if err != nil {
		return nil, err
	}
	f.API.RegisterMetastore(m)
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

	f.metastoreClient = metastoreclient.New(
		f.logger,
		f.Cfg.Metastore.GRPCClientConfig,
		disc,
	)
	return f.metastoreClient.Service(), nil
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

// The shutdown helper utility emerged due to the need to handle request
// draining at the server level.
//
// Since the server is a dependency of many services that handle requests
// and is only shut down after the services have stopped, there's a possibility
// that a de-initialized component may receive requests, which causes undefined
// behaviour.
//
// In other scenarios, request draining could be managed at a higher level,
// such as in a load balancer or the service discovery mechanism. However,
// there's no _reliable_ mechanism to ensure that all the clients are informed
// of the server's shutdown and confirmed that they have stopped sending
// requests to this specific instance.
//
// The helper should be de-initialized first in the dependency chain;
// immediately, it drains the gRPC server, thereby preventing any further
// requests from being processed. THe helper does not affect the HTTP
// server that serves metrics and profiles.
func (f *Phlare) initShutdownHelper() (services.Service, error) {
	shutdownServer := func(error) error {
		if f.Server.GRPC != nil {
			level.Info(f.logger).Log("msg", "shutting down gRPC server")
			f.Server.GRPC.GracefulStop()
		}
		return nil
	}
	return services.NewIdleService(nil, shutdownServer), nil
}
