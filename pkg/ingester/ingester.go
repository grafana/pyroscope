package ingester

import (
	"context"
	"flag"
	"fmt"
	"sync"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingesterv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	phlareobjstore "github.com/grafana/phlare/pkg/objstore"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/pprof"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/usagestats"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/validation"
)

var activeTenantsStats = usagestats.NewInt("ingester_active_tenants")

type Config struct {
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.LifecyclerConfig.RegisterFlags(f, util.Logger)
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

	lifecycler        *ring.Lifecycler
	lifecyclerWatcher *services.FailureWatcher

	storageBucket phlareobjstore.Bucket

	instances    map[string]*instance
	instancesMtx sync.RWMutex

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

func New(phlarectx context.Context, cfg Config, dbConfig phlaredb.Config, storageBucket phlareobjstore.Bucket, limits Limits) (*Ingester, error) {
	i := &Ingester{
		cfg:           cfg,
		phlarectx:     phlarectx,
		logger:        phlarecontext.Logger(phlarectx),
		reg:           phlarecontext.Registry(phlarectx),
		instances:     map[string]*instance{},
		dbConfig:      dbConfig,
		storageBucket: storageBucket,
		limits:        limits,
	}

	var err error
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

	i.lifecyclerWatcher = services.NewFailureWatcher()
	i.lifecyclerWatcher.WatchService(i.lifecycler)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *Ingester) starting(ctx context.Context) error {
	// pass new context to lifecycler, so that it doesn't stop automatically when Ingester's service context is done
	err := i.lifecycler.StartAsync(context.Background())
	if err != nil {
		return err
	}

	err = i.lifecycler.AwaitRunning(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (i *Ingester) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-i.lifecyclerWatcher.Chan(): // handle lifecycler errors
		return fmt.Errorf("lifecycler failed: %w", err)
	}
}

func (i *Ingester) GetOrCreateInstance(tenantID string) (*instance, error) { //nolint:revive
	inst, ok := i.getInstanceByID(tenantID)
	if ok {
		return inst, nil
	}

	i.instancesMtx.Lock()
	defer i.instancesMtx.Unlock()
	inst, ok = i.instances[tenantID]
	if !ok {
		var err error

		inst, err = newInstance(i.phlarectx, i.dbConfig, tenantID, i.storageBucket, NewLimiter(tenantID, i.limits, i.lifecycler, i.cfg.LifecyclerConfig.RingConfig.ReplicationFactor))
		if err != nil {
			return nil, err
		}
		i.instances[tenantID] = inst
		activeTenantsStats.Set(int64(len(i.instances)))
	}
	return inst, nil
}

func (i *Ingester) getInstanceByID(id string) (*instance, bool) {
	i.instancesMtx.RLock()
	defer i.instancesMtx.RUnlock()

	inst, ok := i.instances[id]
	return inst, ok
}

// forInstanceUnary executes the given function for the instance with the given tenant ID in the context.
func forInstanceUnary[T any](ctx context.Context, i *Ingester, f func(*instance) (T, error)) (T, error) {
	var res T
	err := i.forInstance(ctx, func(inst *instance) error {
		r, err := f(inst)
		if err == nil {
			res = r
		}
		return err
	})
	return res, err
}

// forInstance executes the given function for the instance with the given tenant ID in the context.
func (i *Ingester) forInstance(ctx context.Context, f func(*instance) error) error {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	instance, err := i.GetOrCreateInstance(tenantID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return f(instance)
}

func (i *Ingester) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return forInstanceUnary(ctx, i, func(instance *instance) (*connect.Response[pushv1.PushResponse], error) {
		level.Debug(instance.logger).Log("msg", "message received by ingester push")
		for _, series := range req.Msg.Series {
			for _, sample := range series.Samples {
				err := pprof.FromBytes(sample.RawProfile, func(p *profilev1.Profile, size int) error {
					id, err := uuid.Parse(sample.ID)
					if err != nil {
						return err
					}
					if err = instance.Head().Ingest(ctx, p, id, series.Labels...); err != nil {
						reason := validation.ReasonOf(err)
						if reason != validation.Unknown {
							validation.DiscardedProfiles.WithLabelValues(string(reason), instance.tenantID).Add(float64(1))
							validation.DiscardedBytes.WithLabelValues(string(reason), instance.tenantID).Add(float64(size))
							switch validation.ReasonOf(err) {
							case validation.OutOfOrder:
								return connect.NewError(connect.CodeInvalidArgument, err)
							case validation.SeriesLimit:
								return connect.NewError(connect.CodeResourceExhausted, err)
							}
						}
					}
					return err
				})
				if err != nil {
					return nil, err
				}
			}
		}
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})
}

func (i *Ingester) stopping(_ error) error {
	errs := multierror.New()
	errs.Add(services.StopAndAwaitTerminated(context.Background(), i.lifecycler))
	// stop all instances
	i.instancesMtx.RLock()
	defer i.instancesMtx.RUnlock()
	for _, inst := range i.instances {
		errs.Add(inst.Stop())
	}
	return errs.Err()
}

func (i *Ingester) Flush(ctx context.Context, req *connect.Request[ingesterv1.FlushRequest]) (*connect.Response[ingesterv1.FlushResponse], error) {
	i.instancesMtx.RLock()
	defer i.instancesMtx.RUnlock()
	for _, inst := range i.instances {
		if err := inst.Flush(ctx); err != nil {
			return nil, err
		}
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
