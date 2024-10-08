package adaptive_placement

import (
	"context"
	"errors"

	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

const (
	pathRoot      = "adaptive_placement/"
	rulesFilePath = pathRoot + "placement_rules.pb"
	statsFilePath = pathRoot + "distribution_stats.pb"
)

var ErrRulesNotFound = errors.New("placement rules not found")

type StoreReader interface {
	LoadRules(context.Context) (*adaptive_placementpb.PlacementRules, error)
	LoadStats(context.Context) (*adaptive_placementpb.DistributionStats, error)
}

type StoreWriter interface {
	StoreRules(context.Context, *adaptive_placementpb.PlacementRules) error
	StoreStats(context.Context, *adaptive_placementpb.DistributionStats) error
}

type Store interface {
	StoreReader
	StoreWriter
}

type store struct{ bucket objstore.Bucket }

func NewStore(bucket objstore.Bucket) Store { return newStore(bucket) }

func newStore(bucket objstore.Bucket) *store { return &store{bucket: bucket} }

func (s *store) LoadRules(context.Context) (*adaptive_placementpb.PlacementRules, error) {
	return &adaptive_placementpb.PlacementRules{}, nil
}

func (s *store) LoadStats(context.Context) (*adaptive_placementpb.DistributionStats, error) {
	return &adaptive_placementpb.DistributionStats{}, nil
}

func (s *store) StoreRules(context.Context, *adaptive_placementpb.PlacementRules) error {
	return nil
}

func (s *store) StoreStats(context.Context, *adaptive_placementpb.DistributionStats) error {
	return nil
}
