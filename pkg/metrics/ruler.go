package metrics

import (
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const (
	rulesExpiryTime = time.Minute
	rulesRetryTime  = 10 * time.Second
)

type StaticRuler struct {
	overrides *validation.Overrides
}

func NewStaticRulerFromOverrides(overrides *validation.Overrides) Ruler {
	return &StaticRuler{
		overrides: overrides,
	}
}

func (ruler StaticRuler) RecordingRules(tenant string) ([]*model.RecordingRule, error) {
	rules := ruler.overrides.RecordingRules(tenant)
	rs := make([]*model.RecordingRule, 0, len(rules))
	for _, rule := range rules {
		// should never fail, overrides already validated
		r, _ := model.NewRecordingRule(rule)
		rs = append(rs, r)
	}
	return rs, nil
}

// CachedRemoteRuler is a thread-safe ruler that retrieves rules from an external service.
// It has a per-tenant cache: rulesPerTenant
type CachedRemoteRuler struct {
	rulesPerTenant map[string]*tenantCache
	mu             sync.RWMutex

	client RecordingRulesClient

	expiry time.Duration
	retry  time.Duration

	logger log.Logger
}

type RecordingRulesClient interface {
	RecordingRules(tenant string) ([]*model.RecordingRule, error)
}

func NewCachedRemoteRuler(client RecordingRulesClient, logger log.Logger) (Ruler, error) {
	return &CachedRemoteRuler{
		rulesPerTenant: make(map[string]*tenantCache),
		client:         client,
		expiry:         rulesExpiryTime,
		retry:          rulesRetryTime,
		logger:         logger,
	}, nil
}

func (r *CachedRemoteRuler) RecordingRules(tenant string) ([]*model.RecordingRule, error) {
	// get the per-tenant cache
	r.mu.RLock()
	cache, ok := r.rulesPerTenant[tenant]
	r.mu.RUnlock()

	// There's no cache for given tenant: init it
	if !ok {
		r.mu.Lock()
		defer r.mu.Unlock()

		// only race-winner will initialize the per-tenant cache
		cache, ok = r.rulesPerTenant[tenant]
		if !ok {
			cache = &tenantCache{
				initFunc: func() ([]*model.RecordingRule, error) {
					return r.client.RecordingRules(tenant)
				},
				expiry: r.expiry,
				retry:  r.retry,
				logger: r.logger,
			}
			r.rulesPerTenant[tenant] = cache
		}
	}

	// get data from cache:
	return cache.get()
}

// tenantCache is a thread-safe cache that holds an expirable array of rules.
type tenantCache struct {
	value      []*model.RecordingRule
	loaded     bool
	err        error
	refreshing bool
	ttl        time.Time
	expiry     time.Duration
	retry      time.Duration
	initFunc   func() ([]*model.RecordingRule, error)
	mu         sync.RWMutex
	logger     log.Logger
}

// get returns the cached value while it is fresh. Once it expires, the stale
// value keeps being served while a single background refresh runs. Only the
// very first load blocks callers, and it is bounded by the client's timeout.
// A failed fetch is not retried until the retry interval has passed.
func (c *tenantCache) get() ([]*model.RecordingRule, error) {
	c.mu.RLock()
	if time.Now().Before(c.ttl) {
		defer c.mu.RUnlock()
		return c.result()
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.ttl) {
		return c.result()
	}
	if !c.loaded {
		c.apply(c.initFunc())
		return c.result()
	}
	if !c.refreshing {
		c.refreshing = true
		go c.refresh()
	}
	return c.result()
}

func (c *tenantCache) result() ([]*model.RecordingRule, error) {
	if !c.loaded {
		return nil, c.err
	}
	return c.value, nil
}

func (c *tenantCache) refresh() {
	value, err := c.initFunc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshing = false
	c.apply(value, err)
}

func (c *tenantCache) apply(value []*model.RecordingRule, err error) {
	if err != nil {
		level.Error(c.logger).Log("msg", "failed to fetch recording rules", "err", err)
		c.err = err
		c.ttl = time.Now().Add(c.retry)
		return
	}
	c.value = value
	c.loaded = true
	c.ttl = time.Now().Add(c.expiry)
}
