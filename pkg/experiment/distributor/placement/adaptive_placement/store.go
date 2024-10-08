package adaptive_placement

import (
	"context"
	"errors"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

var ErrRulesNotFound = errors.New("placement rules not found")

type StoreReader interface {
	LoadRules(context.Context) (*adaptive_placementpb.PlacementRules, error)
	LoadStats(context.Context, *adaptive_placementpb.DistributionStats) error
}

type StoreWriter interface {
	StoreRules(context.Context, *adaptive_placementpb.PlacementRules) error
	StoreStats(context.Context, *adaptive_placementpb.DistributionStats) error
}

type Store interface {
	StoreReader
	StoreWriter
}
