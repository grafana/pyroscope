package segmentwriter

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	metastoreclient "github.com/grafana/pyroscope/v2/pkg/metastore/client"
	"github.com/grafana/pyroscope/v2/pkg/model/relabel"
	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/segmentwriter/memdb"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/util"
	"github.com/grafana/pyroscope/v2/pkg/util/fieldcategory"
	"github.com/grafana/pyroscope/v2/pkg/util/health"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const (
	RingName = "segment-writer"
	RingKey  = "segment-writer-ring"

	minFlushConcurrency         = 8
	defaultSegmentDuration      = 500 * time.Millisecond
	defaultHedgedRequestMaxRate = 2  // 2 hedged requests per second
	defaultHedgedRequestBurst   = 10 // allow bursts of 10 hedged requests

	// Shares the segments/ prefix so a segments/* bucket policy covers the probe too.
	bucketHealthCheckPrefix = block.DirNameSegment + "/_health/"
)

type Config struct {
	GRPCClientConfig         grpcclient.Config     `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with the segment writer."`
	LifecyclerConfig         ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration          time.Duration         `yaml:"segment_duration,omitempty" category:"advanced"`
	FlushConcurrency         uint                  `yaml:"flush_concurrency,omitempty" category:"advanced"`
	UploadTimeout            time.Duration         `yaml:"upload-timeout,omitempty" category:"advanced"`
	UploadMaxRetries         int                   `yaml:"upload-retry_max_retries,omitempty" category:"advanced"`
	UploadMinBackoff         time.Duration         `yaml:"upload-retry_min_period,omitempty" category:"advanced"`
	UploadMaxBackoff         time.Duration         `yaml:"upload-retry_max_period,omitempty" category:"advanced"`
	UploadHedgeAfter         time.Duration         `yaml:"upload-hedge_upload_after,omitempty" category:"advanced"`
	UploadHedgeRateMax       float64               `yaml:"upload-hedge_rate_max,omitempty" category:"advanced"`
	UploadHedgeRateBurst     uint                  `yaml:"upload-hedge_rate_burst,omitempty" category:"advanced"`
	MetadataDLQEnabled       bool                  `yaml:"metadata_dlq_enabled,omitempty" category:"advanced"`
	MetadataUpdateTimeout    time.Duration         `yaml:"metadata_update_timeout,omitempty" category:"advanced"`
	BucketHealthCheckEnabled bool                  `yaml:"bucket_health_check_enabled,omitempty" category:"advanced"`
	BucketHealthCheckTimeout time.Duration         `yaml:"bucket_health_check_timeout,omitempty" category:"advanced"`
}

func (cfg *Config) Validate() error {
	// TODO(kolesnikovae): implement.
	if err := cfg.LifecyclerConfig.Validate(); err != nil {
		return err
	}
	return cfg.GRPCClientConfig.Validate()
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "segment-writer"
	// Ring/KV store flags come from dskit and cannot be tagged directly; mark them advanced here.
	fieldcategory.AddOverrides(map[string]fieldcategory.Category{
		prefix + ".availability-zone":                  fieldcategory.Advanced,
		prefix + ".consul.hostname":                    fieldcategory.Advanced,
		prefix + ".distributor.replication-factor":     fieldcategory.Advanced,
		prefix + ".distributor.zone-awareness-enabled": fieldcategory.Advanced,
		prefix + ".etcd.endpoints":                     fieldcategory.Advanced,
		prefix + ".etcd.password":                      fieldcategory.Advanced,
		prefix + ".etcd.username":                      fieldcategory.Advanced,
		prefix + ".lifecycler.interface":               fieldcategory.Advanced,
		prefix + ".store":                              fieldcategory.Advanced,
		prefix + ".tokens-file-path":                   fieldcategory.Advanced,
	})
	cfg.LifecyclerConfig.RegisterFlagsWithPrefix(prefix+".", f, util.Logger)
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix(prefix+".grpc-client-config", f)
	f.DurationVar(&cfg.SegmentDuration, prefix+".segment-duration", defaultSegmentDuration, "Timeout when flushing segments to bucket.")
	f.UintVar(&cfg.FlushConcurrency, prefix+".flush-concurrency", 0, "Number of concurrent flushes. Defaults to the number of CPUs, but not less than 8.")
	f.DurationVar(&cfg.UploadTimeout, prefix+".upload-timeout", 2*time.Second, "Timeout for upload requests, including retries.")
	f.IntVar(&cfg.UploadMaxRetries, prefix+".upload-max-retries", 3, "Number of times to backoff and retry before failing.")
	f.DurationVar(&cfg.UploadMinBackoff, prefix+".upload-retry-min-period", 50*time.Millisecond, "Minimum delay when backing off.")
	f.DurationVar(&cfg.UploadMaxBackoff, prefix+".upload-retry-max-period", defaultSegmentDuration, "Maximum delay when backing off.")
	f.DurationVar(&cfg.UploadHedgeAfter, prefix+".upload-hedge-after", defaultSegmentDuration, "Time after which to hedge the upload request.")
	f.Float64Var(&cfg.UploadHedgeRateMax, prefix+".upload-hedge-rate-max", defaultHedgedRequestMaxRate, "Maximum number of hedged requests per second. 0 disables rate limiting.")
	f.UintVar(&cfg.UploadHedgeRateBurst, prefix+".upload-hedge-rate-burst", defaultHedgedRequestBurst, "Maximum number of hedged requests in a burst.")
	f.BoolVar(&cfg.MetadataDLQEnabled, prefix+".metadata-dlq-enabled", true, "Enables dead letter queue (DLQ) for metadata. If the metadata update fails, it will be stored and updated asynchronously.")
	f.DurationVar(&cfg.MetadataUpdateTimeout, prefix+".metadata-update-timeout", 2*time.Second, "Timeout for metadata update requests.")
	f.BoolVar(&cfg.BucketHealthCheckEnabled, prefix+".bucket-health-check-enabled", true, "Uploads and removes a small object at startup to verify bucket write access. Startup fails if the upload fails; a failed removal is only logged.")
	f.DurationVar(&cfg.BucketHealthCheckTimeout, prefix+".bucket-health-check-timeout", 10*time.Second, "Timeout for bucket health check operations.")
}

type Limits interface {
	IngestionRelabelingRules(tenantID string) []*relabel.Config
	DistributorUsageGroups(tenantID string) *validation.UsageGroupConfig
}

type SegmentWriterService struct {
	services.Service
	segmentwriterv1.UnimplementedSegmentWriterServiceServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer
	health health.Service

	requests           util.InflightRequests
	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	storageBucket phlareobj.Bucket
	segmentWriter *segmentsWriter

	bucketHealthCheckCleanupFailures prometheus.Counter
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
	i.bucketHealthCheckCleanupFailures = promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "segment_writer",
		Name:      "bucket_health_check_cleanup_failures_total",
		Help:      "Times the startup bucket health check could not remove its probe object.",
	})

	metrics := newSegmentMetrics(i.reg)
	headMetrics := memdb.NewHeadMetricsWithPrefix(reg, "pyroscope_segment_writer")
	i.segmentWriter = newSegmentWriter(i.logger, metrics, headMetrics, config, limits, storageBucket, metastoreClient)
	i.subservicesWatcher = services.NewFailureWatcher()
	i.subservicesWatcher.WatchManager(i.subservices)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

// performBucketHealthCheck verifies the segment writer can write to object storage before it
// joins the ring. Failure is fatal: a definitive upload error surfaces as codes.Unknown, which
// the client does not retry, so writes to a write-broken instance are lost, not failed over.
func (i *SegmentWriterService) performBucketHealthCheck(ctx context.Context) error {
	if !i.config.BucketHealthCheckEnabled {
		return nil
	}

	name := bucketHealthCheckPrefix + i.config.LifecyclerConfig.ID
	payload := bucketHealthCheckPayload(i.config.LifecyclerConfig.ID)
	level.Debug(i.logger).Log("msg", "starting bucket health check", "object", name)

	uploadCtx, cancelUpload := context.WithTimeout(ctx, i.config.BucketHealthCheckTimeout)
	defer cancelUpload()

	// An already-expired context skips the loop entirely, so err must start out non-nil.
	err := uploadCtx.Err()
	retries := backoff.New(uploadCtx, backoff.Config{
		MinBackoff: i.config.UploadMinBackoff,
		MaxBackoff: i.config.UploadMaxBackoff,
		MaxRetries: i.config.UploadMaxRetries,
	})
	for retries.Ongoing() {
		if err = i.storageBucket.Upload(uploadCtx, name, bytes.NewReader(payload)); err == nil {
			break
		}
		retries.Wait()
	}
	if err != nil {
		level.Error(i.logger).Log("msg", "bucket health check failed", "object", name, "err", err)
		return fmt.Errorf("bucket health check failed: %w", err)
	}

	// Best effort: the key is per instance, so a bucket denying deletes keeps one object per
	// replica, not one per restart.
	deleteCtx, cancelDelete := context.WithTimeout(ctx, i.config.BucketHealthCheckTimeout)
	defer cancelDelete()
	if err := i.storageBucket.Delete(deleteCtx, name); err != nil {
		i.bucketHealthCheckCleanupFailures.Inc()
		level.Warn(i.logger).Log("msg", "failed to remove bucket health check object", "object", name, "err", err)
	}

	level.Debug(i.logger).Log("msg", "bucket health check succeeded")
	return nil
}

func bucketHealthCheckPayload(instanceID string) []byte {
	return []byte(fmt.Sprintf(
		"pyroscope segment-writer bucket health check\ninstance: %s\nwritten: %s\n"+
			"Written at startup to verify write access to the bucket. Safe to delete.\n",
		instanceID, time.Now().UTC().Format(time.RFC3339),
	))
}

func (i *SegmentWriterService) starting(ctx context.Context) error {
	if err := i.performBucketHealthCheck(ctx); err != nil {
		return err
	}

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
	errs := multierror.New()
	errs.Add(services.StopManagerAndAwaitStopped(context.Background(), i.subservices))
	time.Sleep(i.config.LifecyclerConfig.MinReadyDuration)
	i.requests.Drain()
	i.segmentWriter.stop()
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
		segment.ingest(req.TenantId, p.Profile, id, req.Labels, req.Annotations)
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
