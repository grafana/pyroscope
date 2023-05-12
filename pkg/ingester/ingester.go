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
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
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

	lifecycler         *ring.Lifecycler
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

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

	rpEnforcer := newRetentionPolicyEnforcer(phlarecontext.Logger(phlarectx), i, defaultRetentionPolicy(), dbConfig)
	i.subservices, err = services.NewManager(i.lifecycler, rpEnforcer)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
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
	// stop all instances
	i.instancesMtx.RLock()
	defer i.instancesMtx.RUnlock()
	for _, inst := range i.instances {
		errs.Add(inst.Stop())
	}
	return errs.Err()
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

func (i *Ingester) evictBlock(tenantID string, b ulid.ULID, fn func() error) error {
	// We lock instances map for writes to ensure that no new instances are
	// created during the procedure. Otherwise, during initialization, the
	// new PhlareDB instance may try to load a block that has already been
	// deleted, or is being deleted.
	i.instancesMtx.RLock()
	defer i.instancesMtx.RUnlock()
	// The map only contains PhlareDB instances that has been initialized since
	// the process start, therefore there is no guarantee that we will find the
	// discovered candidate block there. If it is the case, we have to ensure that
	// the block won't be accessed, before and during deleting it from the disk.
	if tenantInstance, ok := i.instances[tenantID]; ok {
		if _, err := tenantInstance.Evict(b); err != nil {
			return fmt.Errorf("failed to evict block %s/%s: %w", tenantID, b, err)
		}
	}
	return fn()
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
