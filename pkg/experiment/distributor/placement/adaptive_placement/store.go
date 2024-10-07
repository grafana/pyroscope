package adaptive_placement

import (
	"context"
	"errors"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

var ErrRulesNotFound = errors.New("placement rules not found")

type StoreReader interface {
	LoadStats(context.Context) (*adaptive_placementpb.DistributionStats, error)
	LoadRules(context.Context) (*adaptive_placementpb.PlacementRules, error)
}

type StoreWriter interface {
	StoreStats(context.Context, *adaptive_placementpb.DistributionStats) error
	StoreRules(context.Context, *adaptive_placementpb.PlacementRules) error
}

type Store interface {
	StoreReader
	StoreWriter
}
