package adaptive_placement

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

const (
	pathRoot      = "adaptive_placement/"
	rulesFilePath = pathRoot + "placement_rules.binpb"
	statsFilePath = pathRoot + "placement_stats.binpb"
)

var (
	ErrRulesNotFound = errors.New("placement rules not found")
	ErrStatsNotFound = errors.New("placement stats not found")
)

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

type BucketStore struct{ bucket objstore.Bucket }

func NewStore(bucket objstore.Bucket) *BucketStore { return &BucketStore{bucket: bucket} }

func (s *BucketStore) LoadRules(ctx context.Context) (*adaptive_placementpb.PlacementRules, error) {
	var rules adaptive_placementpb.PlacementRules
	if err := s.get(ctx, rulesFilePath, &rules); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, ErrRulesNotFound
		}
		return nil, err
	}
	return &rules, nil
}

func (s *BucketStore) LoadStats(ctx context.Context) (*adaptive_placementpb.DistributionStats, error) {
	var stats adaptive_placementpb.DistributionStats
	if err := s.get(ctx, statsFilePath, &stats); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, ErrStatsNotFound
		}
		return nil, err
	}
	return &stats, nil
}

func (s *BucketStore) StoreRules(ctx context.Context, rules *adaptive_placementpb.PlacementRules) error {
	return s.put(ctx, rulesFilePath, rules)
}

func (s *BucketStore) StoreStats(ctx context.Context, stats *adaptive_placementpb.DistributionStats) error {
	return s.put(ctx, statsFilePath, stats)
}

type vtProtoMessage interface {
	UnmarshalVT([]byte) error
	MarshalVT() ([]byte, error)
}

func (s *BucketStore) get(ctx context.Context, name string, m vtProtoMessage) error {
	r, err := s.bucket.Get(ctx, name)
	if err != nil {
		return err
	}
	defer func() {
		_ = r.Close()
	}()
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(r); err != nil {
		return err
	}
	return m.UnmarshalVT(buf.Bytes())
}

func (s *BucketStore) put(ctx context.Context, name string, m vtProtoMessage) error {
	b, err := m.MarshalVT()
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, name, bytes.NewReader(b))
}

// EmptyStore is a Store implementation that always returns
// empty rules and stats, and doesn't store anything.
type EmptyStore struct{}

func NewEmptyStore() *EmptyStore { return new(EmptyStore) }

func (e *EmptyStore) LoadRules(context.Context) (*adaptive_placementpb.PlacementRules, error) {
	return &adaptive_placementpb.PlacementRules{CreatedAt: time.Now().UnixNano()}, nil
}

func (e *EmptyStore) LoadStats(context.Context) (*adaptive_placementpb.DistributionStats, error) {
	return &adaptive_placementpb.DistributionStats{CreatedAt: time.Now().UnixNano()}, nil
}

func (e *EmptyStore) StoreRules(context.Context, *adaptive_placementpb.PlacementRules) error {
	return nil
}

func (e *EmptyStore) StoreStats(context.Context, *adaptive_placementpb.DistributionStats) error {
	return nil
}
