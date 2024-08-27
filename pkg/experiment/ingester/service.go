package ingester

import (
	"context"
	"flag"
	"fmt"
	segmentWriterV1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/memdb"
	"time"

	"github.com/google/uuid"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/util"
)

type Config struct {
	GRPCClientConfig grpcclient.Config     `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the segment writer."`
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration  time.Duration         `yaml:"segmentDuration,omitempty"`
	Async            bool                  `yaml:"async,omitempty"` //todo make it pertenant
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "segment-writer."
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix, f)
	cfg.LifecyclerConfig.RegisterFlagsWithPrefix(prefix, f, util.Logger)
	f.DurationVar(&cfg.SegmentDuration, prefix+"segment.duration", 500*time.Millisecond, "Timeout when flushing segments to bucket.")
	f.BoolVar(&cfg.Async, prefix+"async", false, "Enable async mode for segment writer.")
}

func (cfg *Config) Validate() error {
	// TODO(kolesnikovae): implement.
	if err := cfg.LifecyclerConfig.Validate(); err != nil {
		return err
	}
	return cfg.GRPCClientConfig.Validate()
}

type SegmentWriterService struct {
	services.Service
	segmentwriterv1.UnimplementedSegmentWriterServiceServer

	cfg      Config
	dbConfig phlaredb.Config
	logger   log.Logger

	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	storageBucket phlareobj.Bucket

	//limiters      *limiters
	segmentWriter *segmentsWriter

	//limits Limits
	reg prometheus.Registerer
}

type ingesterFlusherCompat struct {
	*SegmentWriterService
}

func (i *ingesterFlusherCompat) Flush() {
	err := i.SegmentWriterService.Flush()
	if err != nil {
		level.Error(i.SegmentWriterService.logger).Log("msg", "flush failed", "err", err)
	}
}

func New(reg prometheus.Registerer, log log.Logger, cfg Config, dbConfig phlaredb.Config, storageBucket phlareobj.Bucket, metastoreClient *metastoreclient.Client) (*SegmentWriterService, error) {

	i := &SegmentWriterService{
		cfg:           cfg,
		logger:        log,
		reg:           reg,
		dbConfig:      dbConfig,
		storageBucket: storageBucket,
	}

	var (
		err error
	)

	i.lifecycler, err = ring.NewLifecycler(
		cfg.LifecyclerConfig,
		&ingesterFlusherCompat{i},
		"segment-writer-ring-name",
		"segment-writer-ring-key",
		true,
		i.logger, prometheus.WrapRegistererWithPrefix("pyroscope_segment_writer_", i.reg))
	if err != nil {
		return nil, err
	}

	i.subservices, err = services.NewManager(i.lifecycler)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	if storageBucket == nil {
		return nil, errors.New("storage bucket is required for segment writer")
	}
	if metastoreClient == nil {
		return nil, errors.New("metastore client is required for segment writer")
	}
	segmentMetrics := newSegmentMetrics(i.reg)
	headMetrics := memdb.NewHeadMetricsWithPrefix(reg, "pyroscope_segment_writer")

	i.segmentWriter = newSegmentWriter(i.logger, segmentMetrics, headMetrics, segmentWriterConfig{
		segmentDuration: cfg.SegmentDuration,
	}, storageBucket, metastoreClient)
	i.subservicesWatcher = services.NewFailureWatcher()
	i.subservicesWatcher.WatchManager(i.subservices)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *SegmentWriterService) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, i.subservices)
}

func (i *SegmentWriterService) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-i.subservicesWatcher.Chan(): // handle lifecycler errors
		return fmt.Errorf("lifecycler failed: %w", err)
	}
}

func (i *SegmentWriterService) stopping(_ error) error {
	errs := multierror.New()
	errs.Add(services.StopManagerAndAwaitStopped(context.Background(), i.subservices))
	errs.Add(i.segmentWriter.Stop())
	return errs.Err()
}

func (i *SegmentWriterService) Push(ctx context.Context, req *connect.Request[segmentWriterV1.PushRequest]) (*connect.Response[segmentWriterV1.PushResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	series := req.Msg.Series
	var shard = shardKey(series.Shard)
	sample := series.Sample
	id, err := uuid.Parse(sample.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	p, err := pprof.RawFromBytes(sample.RawProfile)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	wait := i.segmentWriter.ingest(shard, func(segment segmentIngest) {
		segment.ingest(ctx, tenantID, p.Profile, id, series.Labels)
	})
	if i.cfg.Async {
		return connect.NewResponse(&segmentWriterV1.PushResponse{}), nil
	}
	t1 := time.Now()
	if err = wait.waitFlushed(ctx); err != nil {
		i.segmentWriter.metrics.segmentFlushTimeouts.WithLabelValues(tenantID).Inc()
		i.segmentWriter.metrics.segmentFlushWaitDuration.WithLabelValues(tenantID).Observe(time.Since(t1).Seconds())
		if errors.Is(err, context.DeadlineExceeded) {
			level.Error(i.logger).Log("msg", "flush timeout", "err", err)
		} else {
			level.Error(i.logger).Log("msg", "flush err", "err", err)
		}
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	i.segmentWriter.metrics.segmentFlushWaitDuration.WithLabelValues(tenantID).Observe(time.Since(t1).Seconds())
	return connect.NewResponse(&segmentWriterV1.PushResponse{}), nil
}

func (i *SegmentWriterService) Flush() error {
	return i.segmentWriter.Stop()
}

func (i *SegmentWriterService) TransferOut(ctx context.Context) error {
	return ring.ErrTransferDisabled
}

// CheckReady is used to indicate to k8s when the ingesters are ready for
// the addition removal of another ingester. Returns 204 when the ingester is
// ready, 500 otherwise.
func (i *SegmentWriterService) CheckReady(ctx context.Context) error {
	if s := i.State(); s != services.Running && s != services.Stopping {
		return fmt.Errorf("ingester not ready: %v", s)
	}
	return i.lifecycler.CheckReady(ctx)
}
