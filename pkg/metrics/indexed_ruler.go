package metrics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/objstore"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	settingsrecording "github.com/grafana/pyroscope/v2/pkg/settings/recording"
	"github.com/grafana/pyroscope/v2/pkg/util"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const (
	fetchRetryInterval = 5 * time.Second
	prefetchWorkers    = 8
)

var errFetchBackoff = errors.New("recent failure fetching recording rules")

type IndexedRulerConfig struct {
	RefreshInterval time.Duration
	FetchTimeout    time.Duration
}

// IndexedRuler serves a tenant's recording rules from the overrides plus the
// stored rules in the bucket. Stored rules are only looked up for tenants listed
// in the index that tenant-settings maintains, so any other tenant is answered
// without I/O. The rules of indexed tenants are prefetched whenever the index
// changes; a tenant whose rules are missing or outdated is fetched synchronously.
type IndexedRuler struct {
	services.Service

	bucket    objstore.Bucket
	store     *settingsrecording.IndexStore
	overrides *validation.Overrides
	cfg       IndexedRulerConfig
	logger    log.Logger
	retry     time.Duration

	index atomic.Pointer[settingsrecording.Index]

	mu        sync.RWMutex
	snapshots map[string]*rulesSnapshot
	composed  map[string]*composedRules
	retryAt   map[string]time.Time
	fetches   singleflight.Group

	indexTenants prometheus.Gauge
	indexFailure prometheus.Counter
	fetchTotal   *prometheus.CounterVec
}

type rulesSnapshot struct {
	generation int64
	rules      []*settingsv1.RecordingRuleStore
}

type composedRules struct {
	stored *rulesSnapshot
	config []*settingsv1.RecordingRule
	rules  []*model.RecordingRule
}

func NewIndexedRuler(
	cfg IndexedRulerConfig,
	bucket objstore.Bucket,
	overrides *validation.Overrides,
	logger log.Logger,
	reg prometheus.Registerer,
) *IndexedRuler {
	if bucket == nil {
		bucket = objstore.NewInMemBucket()
		level.Warn(logger).Log("msg", "no storage bucket configured, only recording rules from the configuration will be used")
	}
	r := &IndexedRuler{
		bucket:    bucket,
		store:     settingsrecording.NewIndexStore(bucket),
		overrides: overrides,
		cfg:       cfg,
		logger:    logger,
		retry:     fetchRetryInterval,
		snapshots: make(map[string]*rulesSnapshot),
		composed:  make(map[string]*composedRules),
		retryAt:   make(map[string]time.Time),
		indexTenants: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_index_tenants",
			Help:      "Number of tenants with stored recording rules in the loaded index.",
		}),
		indexFailure: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_index_refresh_failures_total",
			Help:      "Number of failed refreshes of the recording rules index.",
		}),
		fetchTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_fetches_total",
			Help:      "Number of fetches of a tenant's stored recording rules, by result.",
		}, []string{"result"}),
	}
	r.index.Store(&settingsrecording.Index{Tenants: map[string]int64{}})
	r.Service = services.NewTimerService(cfg.RefreshInterval, r.starting, r.refreshNoError, nil)
	return r
}

func (r *IndexedRuler) starting(ctx context.Context) error {
	if err := r.refresh(ctx); err != nil {
		return fmt.Errorf("failed to load recording rules index: %w", err)
	}
	return nil
}

func (r *IndexedRuler) refreshNoError(ctx context.Context) error {
	util.Recover(func() {
		if err := r.refresh(ctx); err != nil {
			r.indexFailure.Inc()
			_ = level.Error(r.logger).Log("msg", "failed to refresh recording rules index", "err", err)
		}
	})
	return nil
}

func (r *IndexedRuler) refresh(ctx context.Context) error {
	idx, err := r.store.Load(ctx)
	switch {
	case err == nil:
	case errors.Is(err, settingsrecording.ErrIndexNotFound):
		idx = &settingsrecording.Index{GeneratedAt: time.Now().UnixNano(), Tenants: map[string]int64{}}
	default:
		return err
	}
	if current := r.index.Load(); current.GeneratedAt > idx.GeneratedAt {
		_ = level.Warn(r.logger).Log(
			"msg", "recording rules index is outdated",
			"discovered", time.Unix(0, idx.GeneratedAt),
			"loaded", time.Unix(0, current.GeneratedAt),
		)
		return nil
	}
	r.index.Store(idx)
	r.indexTenants.Set(float64(len(idx.Tenants)))

	var g errgroup.Group
	g.SetLimit(prefetchWorkers)
	for tenant, generation := range idx.Tenants {
		if r.snapshotOf(tenant, generation) != nil {
			continue
		}
		g.Go(func() error {
			_, _ = r.fetch(tenant, generation)
			return nil
		})
	}
	_ = g.Wait()

	r.forget(idx)
	return nil
}

func (r *IndexedRuler) forget(idx *settingsrecording.Index) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for tenant := range r.snapshots {
		if _, ok := idx.Tenants[tenant]; !ok {
			delete(r.snapshots, tenant)
			delete(r.retryAt, tenant)
			delete(r.composed, tenant)
		}
	}
}

// RecordingRules returns the rules of the tenant. It fails only when the tenant
// has stored rules that could not be read and no earlier version is cached.
func (r *IndexedRuler) RecordingRules(tenant string) ([]*model.RecordingRule, error) {
	config := r.overrides.RecordingRules(tenant)
	generation, indexed := r.index.Load().Tenants[tenant]
	if !indexed && len(config) == 0 {
		return nil, nil
	}

	var stored *rulesSnapshot
	if indexed {
		var err error
		if stored, err = r.storedRules(tenant, generation); err != nil {
			return nil, err
		}
	}
	return r.compose(tenant, stored, config), nil
}

func (r *IndexedRuler) storedRules(tenant string, generation int64) (*rulesSnapshot, error) {
	snapshot, retryAt := r.snapshotAndRetry(tenant)
	if snapshot != nil && snapshot.generation >= generation {
		return snapshot, nil
	}
	if time.Now().Before(retryAt) {
		return r.fallback(snapshot, errFetchBackoff)
	}
	fetched, err := r.fetch(tenant, generation)
	if err != nil {
		return r.fallback(snapshot, err)
	}
	return fetched, nil
}

func (r *IndexedRuler) fallback(previous *rulesSnapshot, err error) (*rulesSnapshot, error) {
	if previous != nil {
		return previous, nil
	}
	return nil, err
}

func (r *IndexedRuler) fetch(tenant string, generation int64) (*rulesSnapshot, error) {
	v, err, _ := r.fetches.Do(tenant, func() (any, error) {
		if s := r.snapshotOf(tenant, generation); s != nil {
			return s, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.FetchTimeout)
		defer cancel()
		stored, err := settingsrecording.ReadStoredRules(ctx, r.bucket, r.logger, tenant)
		if err != nil {
			r.fetchTotal.WithLabelValues("error").Inc()
			level.Error(r.logger).Log("msg", "failed to fetch recording rules", "tenant", tenant, "err", err)
			r.mu.Lock()
			r.retryAt[tenant] = time.Now().Add(r.retry)
			r.mu.Unlock()
			return nil, err
		}
		r.fetchTotal.WithLabelValues("success").Inc()
		s := &rulesSnapshot{
			generation: max(stored.Generation, generation),
			rules:      stored.Rules,
		}
		r.mu.Lock()
		r.snapshots[tenant] = s
		delete(r.retryAt, tenant)
		r.mu.Unlock()
		return s, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*rulesSnapshot), nil
}

func (r *IndexedRuler) snapshotAndRetry(tenant string) (*rulesSnapshot, time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshots[tenant], r.retryAt[tenant]
}

func (r *IndexedRuler) snapshotOf(tenant string, generation int64) *rulesSnapshot {
	s, _ := r.snapshotAndRetry(tenant)
	if s == nil || s.generation < generation {
		return nil
	}
	return s
}

func (r *IndexedRuler) compose(tenant string, stored *rulesSnapshot, config []*settingsv1.RecordingRule) []*model.RecordingRule {
	r.mu.RLock()
	c := r.composed[tenant]
	r.mu.RUnlock()
	if c != nil && c.stored == stored && sameRules(c.config, config) {
		return c.rules
	}

	var storedRules []*settingsv1.RecordingRuleStore
	if stored != nil {
		storedRules = stored.rules
	}
	effective := settingsrecording.EffectiveRules(config, storedRules)
	rules := make([]*model.RecordingRule, 0, len(effective))
	for _, rule := range effective {
		parsed, err := model.NewRecordingRule(rule)
		if err != nil {
			level.Error(r.logger).Log("msg", "failed to parse recording rule", "tenant", tenant, "rule", rule.Id, "err", err)
			continue
		}
		rules = append(rules, parsed)
	}

	r.mu.Lock()
	r.composed[tenant] = &composedRules{stored: stored, config: config, rules: rules}
	r.mu.Unlock()
	return rules
}

func sameRules(a, b []*settingsv1.RecordingRule) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || a[0] == b[0]
}
