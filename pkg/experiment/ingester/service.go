package ingester

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sync"
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
	"github.com/grafana/pyroscope/pkg/validation"
)

const (
	RingName = "segment-writer"
	RingKey  = "segment-writer-ring"
)

type Config struct {
	GRPCClientConfig grpcclient.Config     `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the segment writer."`
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration  time.Duration         `yaml:"segmentDuration,omitempty"`
	Async            bool                  `yaml:"async,omitempty"` //todo make it pertenant
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "segment-writer"
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix, f)
	cfg.LifecyclerConfig.RegisterFlagsWithPrefix(prefix+".", f, util.Logger)
	f.DurationVar(&cfg.SegmentDuration, prefix+".segment.duration", 500*time.Millisecond, "Timeout when flushing segments to bucket.")
	f.BoolVar(&cfg.Async, prefix+".async", false, "Enable async mode for segment writer.")
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

	cfg      Config
	dbConfig phlaredb.Config
	logger   log.Logger
	reg      prometheus.Registerer

	requests           requests
	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	storageBucket phlareobj.Bucket
	segmentWriter *segmentsWriter
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

func New(
	reg prometheus.Registerer,
	log log.Logger,
	cfg Config,
	lim Limits,
	storageBucket phlareobj.Bucket,
	metastoreClient *metastoreclient.Client,
) (*SegmentWriterService, error) {
	i := &SegmentWriterService{
		cfg:           cfg,
		logger:        log,
		reg:           reg,
		storageBucket: storageBucket,
	}

	var err error
	i.lifecycler, err = ring.NewLifecycler(
		cfg.LifecyclerConfig,
		&ingesterFlusherCompat{i},
		RingName,
		RingKey,
		true,
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
	segmentMetrics := newSegmentMetrics(i.reg)
	headMetrics := memdb.NewHeadMetricsWithPrefix(reg, "pyroscope_segment_writer")
	config := segmentWriterConfig{segmentDuration: cfg.SegmentDuration}
	i.segmentWriter = newSegmentWriter(i.logger, segmentMetrics, headMetrics, config, lim, storageBucket, metastoreClient)
	i.subservicesWatcher = services.NewFailureWatcher()
	i.subservicesWatcher.WatchManager(i.subservices)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *SegmentWriterService) starting(ctx context.Context) error {
	if err := services.StartManagerAndAwaitHealthy(ctx, i.subservices); err != nil {
		return err
	}
	i.requests.open()
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
	i.requests.drain()
	errs := multierror.New()
	errs.Add(services.StopManagerAndAwaitStopped(context.Background(), i.subservices))
	errs.Add(i.segmentWriter.Stop())
	return errs.Err()
}

func (i *SegmentWriterService) Push(ctx context.Context, req *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error) {
	if !i.requests.add() {
		return nil, status.Error(codes.Unavailable, "service is unavailable")
	} else {
		defer i.requests.done()
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
		segment.ingest(ctx, req.TenantId, p.Profile, id, req.Labels)
	})
	if i.cfg.Async {
		return &segmentwriterv1.PushResponse{}, nil
	}

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

// The requests utility emerged due to the need to handle request draining at
// the service level.
//
// Ideally, this should be the responsibility of the server using the service.
// However, since the server is a dependency of the service and is only
// shut down after the service is stopped, requests may still arrive
// after the Stop call. This issue arises from how we initialize modules.
//
// In other scenarios, request draining could be managed at a higher level,
// such as in a load balancer or service discovery mechanism. The goal would
// be to stop routing requests to an instance that is about to shut down.
//
// In our case, segment writer service instances are not directly exposed to
// the outside world but are discoverable via the ring (see lifecycler).
// There's no _reliable_ mechanism to ensure that all the ring members are
// aware of fact that the instance is leaving, so requests may continue to
// arrive within a short period of time, until the membership state converges.
type requests struct {
	mu      sync.RWMutex
	wg      sync.WaitGroup
	allowed bool // Indicates if new requests are allowed
}

// open allows new requests to be accepted.
func (r *requests) open() {
	r.mu.Lock()
	r.allowed = true
	r.mu.Unlock()
}

// add increments the WaitGroup if new requests are allowed.
// Returns true if the request was accepted, false otherwise.
func (r *requests) add() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.allowed {
		return false
	}
	r.wg.Add(1)
	return true
}

// done decrements the WaitGroup, indicating a request has completed.
func (r *requests) done() {
	r.wg.Done()
}

// drain prevents new requests from being accepted and waits for
// all ongoing requests to complete.
func (r *requests) drain() {
	r.mu.Lock()
	r.allowed = false
	r.mu.Unlock()
	r.wg.Wait()
}
