package ingester

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/memdb"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/model/relabel"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/health"
	"github.com/grafana/pyroscope/pkg/validation"
)

const (
	RingName = "segment-writer"
	RingKey  = "segment-writer-ring"
)

type Config struct {
	GRPCClientConfig grpcclient.Config     `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the segment writer."`
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration  time.Duration         `yaml:"segment_duration,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "segment-writer"
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix, f)
	cfg.LifecyclerConfig.RegisterFlagsWithPrefix(prefix+".", f, util.Logger)
	f.DurationVar(&cfg.SegmentDuration, prefix+".segment-duration", 500*time.Millisecond, "Timeout when flushing segments to bucket.")
}

func (cfg *Config) Validate() error {
	// TODO(kolesnikovae): implement.
	if err := cfg.LifecyclerConfig.Validate(); err != nil {
		return err
	}
	return cfg.GRPCClientConfig.Validate()
}

type Limits interface {
	IngestionRelabelingRules(tenantID string) []*relabel.Config
	DistributorUsageGroups(tenantID string) *validation.UsageGroupConfig
}

type SegmentWriterService struct {
	services.Service
	segmentwriterv1.UnimplementedSegmentWriterServiceServer

	config   Config
	dbConfig phlaredb.Config
	logger   log.Logger
	reg      prometheus.Registerer
	health   health.Service

	requests           util.InflightRequests
	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	storageBucket phlareobj.Bucket
	segmentWriter *segmentsWriter
}

func New(
	reg prometheus.Registerer,
	logger log.Logger,
	config Config,
	limits Limits,
	health health.Service,
	storageBucket phlareobj.Bucket,
	metastoreClient *metastoreclient.Client,
) (*SegmentWriterService, error) {
	i := &SegmentWriterService{
		config:        config,
		logger:        logger,
		reg:           reg,
		health:        health,
		storageBucket: storageBucket,
	}

	// The lifecycler is only used for discovery: it maintains the state of the
	// instance in the ring and nothing more. Flush is managed explicitly at
	// shutdown, and data/state transfer is not required.
	var err error
	i.lifecycler, err = ring.NewLifecycler(
		config.LifecyclerConfig,
		noOpTransferFlush{},
		RingName,
		RingKey,
		false,
		i.logger, prometheus.WrapRegistererWithPrefix("pyroscope_segment_writer_", i.reg))
	if err != nil {
		return nil, err
	}

	i.subservices, err = services.NewManager(i.lifecycler)
	if err != nil {
		return nil, fmt.Errorf("services manager: %w", err)
	}
	if storageBucket == nil {
		return nil, errors.New("storage bucket is required for segment writer")
	}
	if metastoreClient == nil {
		return nil, errors.New("metastore client is required for segment writer")
	}
	metrics := newSegmentMetrics(i.reg)
	headMetrics := memdb.NewHeadMetricsWithPrefix(reg, "pyroscope_segment_writer")
	i.segmentWriter = newSegmentWriter(i.logger, metrics, headMetrics, config, limits, storageBucket, metastoreClient)
	i.subservicesWatcher = services.NewFailureWatcher()
	i.subservicesWatcher.WatchManager(i.subservices)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *SegmentWriterService) starting(ctx context.Context) error {
	if err := services.StartManagerAndAwaitHealthy(ctx, i.subservices); err != nil {
		return err
	}
	// The instance is ready to handle incoming requests.
	// We do not have to wait for the lifecycler: its readiness check
	// is only used to limit the number of instances that can be coming
	// or going at any one time, by only returning true if all instances
	// are active.
	i.requests.Open()
	i.health.SetServing()
	return nil
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
	i.health.SetNotServing()
	i.requests.Drain()
	errs := multierror.New()
	errs.Add(services.StopManagerAndAwaitStopped(context.Background(), i.subservices))
	errs.Add(i.segmentWriter.Stop())
	return errs.Err()
}

func (i *SegmentWriterService) Push(ctx context.Context, req *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error) {
	if !i.requests.Add() {
		return nil, status.Error(codes.Unavailable, "service is unavailable")
	} else {
		defer func() {
			i.requests.Done()
		}()
	}

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, tenant.ErrNoTenantID.Error())
	}
	var id uuid.UUID
	if err := id.UnmarshalBinary(req.ProfileId); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	p, err := pprof.RawFromBytes(req.Profile)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	wait := i.segmentWriter.ingest(shardKey(req.Shard), func(segment segmentIngest) {
		segment.ingest(req.TenantId, p.Profile, id, req.Labels)
	})

	flushStarted := time.Now()
	defer func() {
		i.segmentWriter.metrics.segmentFlushWaitDuration.
			WithLabelValues(req.TenantId).
			Observe(time.Since(flushStarted).Seconds())
	}()
	if err = wait.waitFlushed(ctx); err == nil {
		return &segmentwriterv1.PushResponse{}, nil
	}

	switch {
	case errors.Is(err, context.Canceled):
		return nil, status.FromContextError(err).Err()

	case errors.Is(err, context.DeadlineExceeded):
		i.segmentWriter.metrics.segmentFlushTimeouts.WithLabelValues(req.TenantId).Inc()
		level.Error(i.logger).Log("msg", "flush timeout", "err", err)
		return nil, status.FromContextError(err).Err()

	case errors.Is(err, ErrMetastoreDLQFailed):
		// This error will cause retry.
		level.Error(i.logger).Log("msg", "failed to store metadata", "err", err)
		return nil, status.Error(codes.Unavailable, err.Error())

	default:
		level.Error(i.logger).Log("msg", "flush err", "err", err)
		return nil, status.Error(codes.Unknown, err.Error())
	}
}

// CheckReady is used to indicate when the ingesters are ready for
// the addition removal of another ingester. Returns 204 when the ingester is
// ready, 500 otherwise.
func (i *SegmentWriterService) CheckReady(ctx context.Context) error {
	if s := i.State(); s != services.Running && s != services.Stopping {
		return fmt.Errorf("ingester not ready: %v", s)
	}
	return i.lifecycler.CheckReady(ctx)
}

type noOpTransferFlush struct{}

func (noOpTransferFlush) Flush()                            {}
func (noOpTransferFlush) TransferOut(context.Context) error { return ring.ErrTransferDisabled }
