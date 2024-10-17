package adaptive_placement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/util"
)

type Agent struct {
	service services.Service
	logger  log.Logger
	store   StoreReader
	config  Config
	metrics *agentMetrics

	placement *AdaptivePlacement
	rules     *adaptive_placementpb.PlacementRules
}

func NewAgent(
	logger log.Logger,
	reg prometheus.Registerer,
	config Config,
	limits Limits,
	store Store,
) *Agent {
	a := Agent{
		logger:    logger,
		store:     store,
		config:    config,
		placement: NewAdaptivePlacement(limits),
		metrics:   newAgentMetrics(reg),
	}
	a.service = services.NewTimerService(
		config.PlacementUpdateInterval,
		a.starting,
		a.loadRulesNoError,
		a.stopping,
	)
	return &a
}

func (a *Agent) Service() services.Service { return a.service }

func (a *Agent) Placement() *AdaptivePlacement { return a.placement }

func (a *Agent) starting(ctx context.Context) error {
	_ = a.loadRulesNoError(ctx)
	if a.rules == nil {
		// The exact reason is logged in loadRules.
		return fmt.Errorf("failed to load placement rules")
	}
	return nil
}

func (a *Agent) stopping(error) error { return nil }

// The function is only needed to satisfy the services.OneIteration
// signature: there's no case when the service stops on its own:
// it's better to serve outdated rules than to not serve at all.
func (a *Agent) loadRulesNoError(ctx context.Context) error {
	util.Recover(func() { a.loadRules(ctx) })
	return nil
}

func (a *Agent) loadRules(ctx context.Context) {
	rules, err := a.store.LoadRules(ctx)
	switch {
	case err == nil:
	case errors.Is(err, ErrRulesNotFound):
		_ = level.Warn(a.logger).Log("msg", "placement rules not found")
		rules = &adaptive_placementpb.PlacementRules{CreatedAt: time.Now().UnixNano()}
	default:
		_ = level.Error(a.logger).Log("msg", "failed to load placement rules", "err", err)
		return
	}
	a.metrics.lag.Set(max(0, time.Since(time.Unix(0, rules.CreatedAt)).Seconds()))
	if a.rules != nil {
		if rules.CreatedAt < a.rules.CreatedAt {
			_ = level.Warn(a.logger).Log(
				"msg", "placement rules are outdated",
				"discovered", time.Unix(0, rules.CreatedAt),
				"loaded", time.Unix(0, a.rules.CreatedAt),
			)
			return
		}
	}
	_ = level.Debug(a.logger).Log(
		"msg", "loading placement rules",
		"created_at", time.Unix(0, rules.CreatedAt),
	)
	a.placement.Update(rules)
	a.rules = rules
}
