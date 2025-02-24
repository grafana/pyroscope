package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

const (
	envVarRecordingRules = "PYROSCOPE_RECORDING_RULES"
	rulesExpiryTime      = time.Minute
)

type StaticRuler struct {
	rules  map[string][]*model.RecordingRule
	logger log.Logger
}

func NewStaticRulerFromEnvVars(logger log.Logger) (Ruler, error) {
	jsonRules := os.Getenv(envVarRecordingRules)

	var rulesByTenant map[string][]*settingsv1.RecordingRule
	if err := json.Unmarshal([]byte(jsonRules), &rulesByTenant); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recording rules: %w", err)
	}

	ruler := &StaticRuler{
		rules:  make(map[string][]*model.RecordingRule, len(rulesByTenant)),
		logger: logger,
	}
	for tenant, rules := range rulesByTenant {
		rs := make([]*model.RecordingRule, 0, len(rules))
		for _, rule := range rules {
			r, err := model.NewRecordingRule(rule)
			if err == nil {
				rs = append(rs, r)
			} else {
				level.Error(logger).Log("msg", "failed to parse recording rule", "rule", rule, "err", err)
			}
		}
		ruler.rules[tenant] = rs
	}
	return ruler, nil
}

func (r StaticRuler) RecordingRules(tenant string) []*model.RecordingRule {
	return r.rules[tenant]
}

// TenantSettingsRuler is a thread-safe ruler that retrieves rules from tenant-settings service.
// It has a per-tenant cache: rulesPerTenant
type TenantSettingsRuler struct {
	rulesPerTenant map[string]*tenantCache
	mu             sync.RWMutex

	client settingsv1.SettingsServiceClient
}

func NewTenantSettingsRuler(client settingsv1.SettingsServiceClient) (Ruler, error) {
	return &TenantSettingsRuler{
		rulesPerTenant: make(map[string]*tenantCache),
		client:         client,
	}, nil
}

func (r *TenantSettingsRuler) RecordingRules(tenant string) []*model.RecordingRule {
	// get the per-tenant cache
	r.mu.RLock()
	rules, ok := r.rulesPerTenant[tenant]
	r.mu.RUnlock()

	// There's no cache for given tenant: init it
	if !ok {
		r.mu.Lock()
		defer r.mu.Unlock()

		// only race-winner will initialize the per-tenant cache
		rules, ok = r.rulesPerTenant[tenant]
		if !ok {
			rules = &tenantCache{
				initFunc: func() []*model.RecordingRule {
					return r.fetchRecordingRules(tenant)
				},
			}
			r.rulesPerTenant[tenant] = rules
		}
	}

	// get data from cache:
	return rules.get()
}

func (r *TenantSettingsRuler) fetchRecordingRules(tenant string) []*model.RecordingRule {
	// TODO missing client func
	// get and parse rules from r.client.GetRecordingRules()
	return []*model.RecordingRule{}
}

// tenantCache is a thread-safe cache that holds an expirable array of rules.
type tenantCache struct {
	value    []*model.RecordingRule
	ttl      time.Time
	initFunc func() []*model.RecordingRule
	mu       sync.Mutex
}

// get returns the stored value if present and not expired.
// Otherwise, a single call to initFunc will be performed to retrieve the value and hold it for future calls within
// the ttl.
func (c *tenantCache) get() []*model.RecordingRule {
	if c.value != nil && time.Now().Before(c.ttl) {
		// value exists and didn't expired
		return c.value
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// only race-winner will fetch the data
	if c.value == nil || time.Now().After(c.ttl) {
		c.value = c.initFunc()
		c.ttl = time.Now().Add(rulesExpiryTime)
	}
	return c.value
}
