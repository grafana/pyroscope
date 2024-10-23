package adaptive_placement

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/util"
)

// Manager maintains placement rules and distribution stats in the store.
//
// Manager implements services.Service interface for convenience, but it's
// meant to be started and stopped explicitly via Start and Stop calls.
//
// If manager is being stopped while updating rules, an ongoing attempt is
// not aborted: we're interested in finishing the operation so that the rules
// reflect the most recent statistics. Another reason is that another instance
// might be already running at the Stop call time.
//
// When just started, the manager may not have enough statistics to build
// the rules: StatsConfidencePeriod should expire before the first update.
// Note that ruler won't downscale datasets for a certain period of time
// after the ruler is created regardless of the confidence period. Therefore,
// it's generally safe to publish rules even with incomplete statistics;
// however, this allows for delays in response to changes of the data flow.
type Manager struct {
	started   atomic.Bool
	startedAt time.Time

	service services.Service
	logger  log.Logger
	config  Config
	limits  Limits
	metrics *managerMetrics

	store Store
	stats *DistributionStats
	ruler *Ruler
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

func (m *Manager) RecordStats(samples iter.Iterator[Sample]) { m.stats.RecordStats(samples) }

func (m *Manager) Start() { m.started.Store(true) }
func (m *Manager) Stop()  { m.started.Store(false) }

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
	if !m.started.Load() {
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

	if time.Since(m.startedAt) < m.config.StatsConfidencePeriod {
		_ = level.Debug(m.logger).Log("msg", "confidence period not expired, skipping update")
		return
	}

	if err := m.store.StoreRules(ctx, rules); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to store placement rules", "err", err)
	} else {
		m.metrics.lastUpdate.SetToCurrentTime()
		_ = level.Debug(m.logger).Log(
			"msg", "placement rules updated",
			"datasets", len(rules.Datasets),
			"created_at", time.Unix(0, rules.CreatedAt),
		)
	}

	if err := m.store.StoreStats(ctx, stats); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to store stats", "err", err)
	} else {
		_ = level.Debug(m.logger).Log(
			"msg", "placement stats updated",
			"datasets", len(rules.Datasets),
			"created_at", time.Unix(0, rules.CreatedAt),
		)
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
	switch {
	case err == nil:
	case errors.Is(err, ErrRulesNotFound):
		_ = level.Warn(m.logger).Log("msg", "placement rules not found")
		rules = &adaptive_placementpb.PlacementRules{CreatedAt: time.Now().UnixNano()}
	default:
		_ = level.Error(m.logger).Log("msg", "failed to load placement rules", "err", err)
		return false
	}
	if m.ruler == nil {
		m.ruler = NewRuler(m.limits)
		m.startedAt = time.Now()
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
				strconv.Itoa(int(dataset.LoadBalancing))).
				Set(float64(dataset.DatasetShardLimit))
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
