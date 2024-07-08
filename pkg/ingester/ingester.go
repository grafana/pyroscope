package ingester

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/uuid"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
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

	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/util"
)

//var activeTenantsStats = usagestats.NewInt("ingester_active_tenants")

type Config struct {
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
	SegmentDuration  time.Duration         `yaml:"segmentDuration,omitempty"`
	Async            bool                  `yaml:"async,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.LifecyclerConfig.RegisterFlags(f, util.Logger)
	f.DurationVar(&cfg.SegmentDuration, "ingester.segment.duration", 1*time.Second, "Timeout when flushing segments to bucket.")
	f.BoolVar(&cfg.Async, "ingester.async", false, "Enable async mode for ingester.")
}

func (cfg *Config) Validate() error {
	return nil
}

type Ingester struct {
	services.Service

	cfg       Config
	dbConfig  phlaredb.Config
	logger    log.Logger
	phlarectx context.Context

	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	storageBucket phlareobj.Bucket

	limiters      *limiters
	segmentWriter *segmentsWriter

	limits Limits
	reg    prometheus.Registerer
}

type ingesterFlusherCompat struct {
	*Ingester
}

func (i *ingesterFlusherCompat) Flush() {
	_, err := i.Ingester.Flush(context.TODO(), connect.NewRequest(&ingesterv1.FlushRequest{}))
	if err != nil {
		level.Error(i.Ingester.logger).Log("msg", "flush failed", "err", err)
	}
}

func New(phlarectx context.Context, cfg Config, dbConfig phlaredb.Config, storageBucket phlareobj.Bucket, limits Limits, queryStoreAfter time.Duration, metastoreClient *metastoreclient.Client) (*Ingester, error) {
	i := &Ingester{
		cfg:           cfg,
		phlarectx:     phlarectx,
		logger:        phlarecontext.Logger(phlarectx),
		reg:           phlarecontext.Registry(phlarectx),
		dbConfig:      dbConfig,
		storageBucket: storageBucket,
		limits:        limits,
	}

	// initialise the local bucket client
	var (
		err error
	)

	i.lifecycler, err = ring.NewLifecycler(
		cfg.LifecyclerConfig,
		&ingesterFlusherCompat{i},
		"ingester",
		"ring",
		true,
		i.logger, prometheus.WrapRegistererWithPrefix("pyroscope_", i.reg))
	if err != nil {
		return nil, err
	}

	i.subservices, err = services.NewManager(i.lifecycler)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	i.limiters = newLimiters(i.limitsForTenant)
	if storageBucket == nil {
		return nil, errors.New("storage bucket is required for segment writer")
	}
	if metastoreClient == nil {
		return nil, errors.New("metastore client is required for segment writer")
	}
	i.segmentWriter = newSegmentWriter(i.phlarectx, i.logger, i.dbConfig, i.limiters, storageBucket, cfg.SegmentDuration, metastoreClient)
	i.subservicesWatcher = services.NewFailureWatcher()
	i.subservicesWatcher.WatchManager(i.subservices)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *Ingester) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, i.subservices)
}

func (i *Ingester) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-i.subservicesWatcher.Chan(): // handle lifecycler errors
		return fmt.Errorf("lifecycler failed: %w", err)
	}
}

func (i *Ingester) stopping(_ error) error {
	errs := multierror.New()
	errs.Add(services.StopManagerAndAwaitStopped(context.Background(), i.subservices))
	errs.Add(i.segmentWriter.Stop())
	return errs.Err()
}

func (i *Ingester) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var waits = make([]segmentWaitFlushed, len(req.Msg.Series))
	for _, series := range req.Msg.Series {
		var shard shardKey = 0
		if series.Shard != nil {
			shard = shardKey(*series.Shard)
		}
		wait, err := i.segmentWriter.ingest(shard, func(segment segmentIngest) error {
			for _, sample := range series.Samples {
				id, err := uuid.Parse(sample.ID)
				if err != nil {
					return err
				}
				err = pprof.FromBytes(sample.RawProfile, func(p *profilev1.Profile, size int) error {
					if err = segment.ingest(ctx, tenantID, p, id, series.Labels...); err != nil {
						reason := validation.ReasonOf(err)
						if reason != validation.Unknown {
							validation.DiscardedProfiles.WithLabelValues(string(reason), tenantID).Add(float64(1))
							validation.DiscardedBytes.WithLabelValues(string(reason), tenantID).Add(float64(size))
							switch validation.ReasonOf(err) {
							case validation.SeriesLimit:
								return connect.NewError(connect.CodeResourceExhausted, err)
							}
						}
					}
					return nil
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		waits = append(waits, wait)
	}
	if i.cfg.Async {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}
	for _, wait := range waits {
		if err = wait.waitFlushed(ctx); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

func (i *Ingester) Flush(ctx context.Context, req *connect.Request[ingesterv1.FlushRequest]) (*connect.Response[ingesterv1.FlushResponse], error) {
	err := i.segmentWriter.Flush(ctx)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&ingesterv1.FlushResponse{}), nil
}

func (i *Ingester) TransferOut(ctx context.Context) error {
	return ring.ErrTransferDisabled
}

// CheckReady is used to indicate to k8s when the ingesters are ready for
// the addition removal of another ingester. Returns 204 when the ingester is
// ready, 500 otherwise.
func (i *Ingester) CheckReady(ctx context.Context) error {
	if s := i.State(); s != services.Running && s != services.Stopping {
		return fmt.Errorf("ingester not ready: %v", s)
	}
	return i.lifecycler.CheckReady(ctx)
}

func (i *Ingester) limitsForTenant(tenantID string) Limiter {
	return NewLimiter(tenantID, i.limits, i.lifecycler,
		1, // i.cfg.LifecyclerConfig.RingConfig.ReplicationFactor
	)
}
