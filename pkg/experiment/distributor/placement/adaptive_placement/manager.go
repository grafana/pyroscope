package adaptive_placement

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
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
		logger: logger,
		config: config,
		limits: limits,
		store:  store,
		stats:  NewDistributionStats(config.StatsAggregationWindow),
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
	m.updateRules(ctx)
	return nil
}

func (m *Manager) updateRules(ctx context.Context) {
	started := m.started.Load()
	if started < 0 {
		// Note that we only reset the ruler here, but not the stats:
		// there's no harm in old samples as long as they are within
		// the retention period.
		m.ruler = nil
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
