package adaptive_placement

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

type Agent struct {
	service services.Service
	logger  log.Logger
	store   StoreReader
	config  Config
	limits  Limits
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
		logger:  logger,
		store:   store,
		config:  config,
		limits:  limits,
		metrics: newAgentMetrics(reg),
	}
	a.service = services.NewTimerService(
		config.PlacementUpdateInterval,
		a.starting,
		a.updatePlacementNoError,
		a.stopping,
	)
	return &a
}

func (a *Agent) Service() services.Service { return a.service }

func (a *Agent) Placement() *AdaptivePlacement { return a.placement }

func (a *Agent) starting(ctx context.Context) error {
	a.updatePlacement(ctx)
	if a.placement == nil {
		// The exact reason is logged in updatePlacement.
		return fmt.Errorf("failed to load placement rules")
	}
	return nil
}

func (a *Agent) stopping(error) error { return nil }

// The function is only needed to satisfy the services.OneIteration
// signature: there's no case when the service stops on its own:
// it's better to serve outdated rules than to not serve at all.
func (a *Agent) updatePlacementNoError(ctx context.Context) error {
	a.updatePlacement(ctx)
	return nil
}

func (a *Agent) updatePlacement(ctx context.Context) {
	rules, err := a.store.LoadRules(ctx)
	if err != nil {
		_ = level.Error(a.logger).Log("msg", "failed to load placement rules", "err", err)
		return
	}
	if a.rules != nil {
		if rules.CreatedAt < a.rules.CreatedAt {
			_ = level.Warn(a.logger).Log(
				"msg", "placement rules are outdated",
				"created_at", time.Unix(0, rules.CreatedAt))
			return
		}
	}
	a.metrics.lag.Set(max(0, time.Since(time.Unix(0, rules.CreatedAt)).Seconds()))
	if a.placement == nil {
		a.placement = NewAdaptivePlacement(a.limits)
	}
	a.placement.Update(rules)
	a.rules = rules
}
