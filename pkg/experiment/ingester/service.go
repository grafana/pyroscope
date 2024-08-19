package ingester

import (
	"context"
	"flag"
	"fmt"
	segmentWriterV1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"time"

	"github.com/google/uuid"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/validation"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/util"
)

type Config struct {
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration  time.Duration         `yaml:"segmentDuration,omitempty"`
	Async            bool                  `yaml:"async,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "segment-writer."
	cfg.LifecyclerConfig.RegisterFlagsWithPrefix(prefix, f, util.Logger)
	f.DurationVar(&cfg.SegmentDuration, prefix+"segment.duration", 500*time.Millisecond, "Timeout when flushing segments to bucket.")
	f.BoolVar(&cfg.Async, prefix+"async", false, "Enable async mode for segment writer.")
}

func (cfg *Config) Validate() error {
	return nil
}

type SegmentWriterService struct {
	services.Service

	cfg       Config
	dbConfig  phlaredb.Config
	logger    log.Logger
	phlarectx context.Context

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

func New(phlarectx context.Context, cfg Config, dbConfig phlaredb.Config, storageBucket phlareobj.Bucket, metastoreClient *metastoreclient.Client) (*SegmentWriterService, error) {
	reg := phlarecontext.Registry(phlarectx)
	reg = prometheus.WrapRegistererWith(prometheus.Labels{"component": "segment-writer"}, reg)
	phlarectx = phlarecontext.WithRegistry(phlarectx, reg)
	log := phlarecontext.Logger(phlarectx)
	phlarectx = phlaredb.ContextWithHeadMetrics(phlarectx, reg, "pyroscope_segment_writer")

	i := &SegmentWriterService{
		cfg:           cfg,
		phlarectx:     phlarectx,
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
	metrics := newSegmentMetrics(i.reg)

	i.segmentWriter = newSegmentWriter(i.phlarectx, i.logger, metrics, i.dbConfig, storageBucket, cfg.SegmentDuration, metastoreClient)
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
	wait, err := i.segmentWriter.ingest(shard, func(segment segmentIngest) error {
		return i.ingestToSegment(ctx, segment, series, tenantID)
	})
	if err != nil {
		return nil, err
	}
	if i.cfg.Async {
		return connect.NewResponse(&segmentWriterV1.PushResponse{}), nil
	}
	t1 := time.Now()
	if err = wait.waitFlushed(ctx); err != nil {
		i.segmentWriter.metrics.segmentFlushTimeouts.WithLabelValues(tenantID).Inc()
		i.segmentWriter.metrics.segmentFlushWaitDuration.WithLabelValues(tenantID).Observe(time.Since(t1).Seconds())
		level.Error(i.logger).Log("msg", "flush timeout", "err", err)
		return nil, err
	}
	i.segmentWriter.metrics.segmentFlushWaitDuration.WithLabelValues(tenantID).Observe(time.Since(t1).Seconds())
	return connect.NewResponse(&segmentWriterV1.PushResponse{}), nil
}

func (i *SegmentWriterService) ingestToSegment(ctx context.Context, segment segmentIngest, series *segmentWriterV1.RawProfileSeries, tenantID string) error {
	for _, sample := range series.Samples {
		id, err := uuid.Parse(sample.ID)
		if err != nil {
			return err
		}
		err = pprof.FromBytes(sample.RawProfile, func(p *profilev1.Profile, size int) error {
			if err = segment.ingest(ctx, tenantID, p, id, series.Labels); err != nil {
				reason := validation.ReasonOf(err)
				if reason != validation.Unknown {
					validation.DiscardedProfiles.WithLabelValues(string(reason), tenantID).Add(float64(1))
					validation.DiscardedBytes.WithLabelValues(string(reason), tenantID).Add(float64(size))
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
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
