package adaptive_placement

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/util"
)

// Manager maintains placement rules and distribution stats in the store.
//
// Manager implements services.Service interface for convenience, but it's
// meant to be started and stopped explicitly via Start and Stop calls.
type Manager struct {
	started atomic.Int64

	service services.Service
	logger  log.Logger
	config  Config
	limits  Limits
	metrics *managerMetrics

	store Store
	ruler *Ruler
	stats *DistributionStats
}

func NewManager(
	logger log.Logger,
	reg prometheus.Registerer,
	config Config,
	limits Limits,
	store Store,
) *Manager {
	m := &Manager{
		logger:  logger,
		config:  config,
		limits:  limits,
		store:   store,
		stats:   NewDistributionStats(config.StatsAggregationWindow),
		metrics: newManagerMetrics(reg),
	}
	m.service = services.NewTimerService(
		config.PlacementUpdateInterval,
		m.starting,
		m.updateRulesNoError,
		m.stopping,
	)
	return m
}

func (m *Manager) Service() services.Service { return m.service }

func (m *Manager) Stats() *DistributionStats { return m.stats }

func (m *Manager) Start() { m.started.Store(time.Now().UnixNano()) }
func (m *Manager) Stop()  { m.started.Store(-1) }

func (m *Manager) starting(context.Context) error { return nil }
func (m *Manager) stopping(error) error           { return nil }

// The function is only needed to satisfy the services.OneIteration
// signature: there's no case when the service stops on its own:
// it's better to serve outdated rules than to not serve at all.
func (m *Manager) updateRulesNoError(ctx context.Context) error {
	util.Recover(func() { m.updateRules(ctx) })
	return nil
}

func (m *Manager) updateRules(ctx context.Context) {
	started := m.started.Load()
	if started < 0 {
		m.reset()
		return
	}
	// Initialize the ruler if it's the first run after start.
	if m.ruler == nil && !m.loadRules(ctx) {
		return
	}

	// Cleanup outdated data first: note that when we load the
	// rules from the store we don't check how old they are.
	now := time.Now()
	m.ruler.Expire(now.Add(-m.config.PlacementRetentionPeriod))
	m.stats.Expire(now.Add(-m.config.StatsRetentionPeriod))

	stats := m.stats.Build()
	rules := m.ruler.BuildRules(stats)

	m.metrics.rulesTotal.Set(float64(len(rules.Datasets)))
	m.metrics.statsTotal.Set(float64(len(stats.Datasets)))

	if time.Since(time.Unix(0, started)) < m.config.StatsConfidencePeriod {
		// Although, we have enough data to build the rules, we may want
		// to wait a bit longer to ensure that the stats are stable.
		// Note that ruler won't downscale datasets for a certain period
		// of time after the ruler is created regardless of this check.
		// Therefore, it's generally safe to skip it.
		return
	}

	if err := m.store.StoreRules(ctx, rules); err != nil {
		m.logger.Log("msg", "failed to store placement rules", "err", err)
	}

	m.metrics.lastUpdate.SetToCurrentTime()
	if err := m.store.StoreStats(ctx, stats); err != nil {
		m.logger.Log("msg", "failed to store stats", "err", err)
	}

	m.exportMetrics(rules, stats)
}

func (m *Manager) reset() {
	// Note that we only reset the ruler here, but not the stats:
	// there's no harm in old samples as long as they are within
	// the retention period.
	m.ruler = nil
	m.metrics.rulesTotal.Set(0)
	m.metrics.statsTotal.Set(0)
	m.metrics.datasetShardLimit.Reset()
	m.metrics.datasetShardUsage.Reset()
	m.metrics.datasetShardUsageBreakdown.Reset()
}

func (m *Manager) loadRules(ctx context.Context) bool {
	rules, err := m.store.LoadRules(ctx)
	if err != nil {
		if !errors.Is(err, ErrRulesNotFound) {
			m.logger.Log("msg", "failed to load placement rules", "err", err)
			return false
		}
	}
	if m.ruler == nil {
		m.ruler = NewRuler(m.limits)
	}
	if rules == nil {
		rules = &adaptive_placementpb.PlacementRules{
			CreatedAt: time.Now().UnixNano(),
		}
	}
	m.ruler.Load(rules)
	return true
}

func (m *Manager) exportMetrics(
	rules *adaptive_placementpb.PlacementRules,
	stats *adaptive_placementpb.DistributionStats,
) {
	if m.config.ExportShardLimitMetrics {
		for _, dataset := range rules.Datasets {
			m.metrics.datasetShardLimit.WithLabelValues(
				rules.Tenants[dataset.Tenant].TenantId,
				dataset.Name,
				strconv.Itoa(int(dataset.Limits.LoadBalancing))).
				Set(float64(dataset.Limits.DatasetShardLimit))
		}
	}

	if m.config.ExportShardUsageMetrics {
		for _, dataset := range stats.Datasets {
			m.metrics.datasetShardUsage.WithLabelValues(
				stats.Tenants[dataset.Tenant].TenantId,
				dataset.Name).
				Set(float64(sum(dataset.Usage)))
		}
	}

	if m.config.ExportShardUsageBreakdownMetrics {
		for _, dataset := range stats.Datasets {
			for i, ds := range dataset.Shards {
				m.metrics.datasetShardUsageBreakdown.WithLabelValues(
					stats.Tenants[dataset.Tenant].TenantId,
					dataset.Name,
					strconv.Itoa(int(stats.Shards[ds].Id)),
					stats.Shards[ds].Owner).
					Set(float64(dataset.Usage[i]))
			}
		}
	}
}
