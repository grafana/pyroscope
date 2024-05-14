package querier

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

type blockInfo struct {
	i typesv1.BlockInfo
}

func newBlockInfo(ulid string) *blockInfo {
	return &blockInfo{
		i: typesv1.BlockInfo{
			Ulid: ulid,
			Compaction: &typesv1.BlockCompaction{
				Level: 1,
			},
		},
	}
}

func (b *blockInfo) withMinTime(minT time.Time, d time.Duration) *blockInfo {
	b.i.MinTime = int64(model.TimeFromUnixNano(minT.UnixNano()))
	b.i.MaxTime = int64(model.TimeFromUnixNano(minT.Add(d).UnixNano()))
	return b
}

func (b *blockInfo) withCompactionLevel(i int32) *blockInfo {
	b.i.Compaction.Level = i
	return b
}

func (b *blockInfo) withCompactionSources(sources ...string) *blockInfo {
	b.i.Compaction.Sources = sources
	return b
}

func (b *blockInfo) withCompactionParents(parents ...string) *blockInfo {
	b.i.Compaction.Parents = parents
	return b
}

func (b *blockInfo) withLabelValue(k, v string) *blockInfo {
	b.i.Labels = append(b.i.Labels, &typesv1.LabelPair{
		Name:  k,
		Value: v,
	})
	return b
}

func (b *blockInfo) withCompactorShard(shard, shardsCount uint64) *blockInfo {
	return b.withLabelValue(
		sharding.CompactorShardIDLabel,
		sharding.FormatShardIDLabelValue(shard, shardsCount),
	)
}

func (b *blockInfo) info() *typesv1.BlockInfo {
	return &b.i
}

type validatorFunc func(t *testing.T, plan map[string]*blockPlanEntry)

func validatePlanBlockIDs(expBlockIDs ...string) validatorFunc {
	return func(t *testing.T, plan map[string]*blockPlanEntry) {
		var blockIDs []string
		for _, planEntry := range plan {
			blockIDs = append(blockIDs, planEntry.Ulids...)
		}
		sort.Strings(blockIDs)
		require.Equal(t, expBlockIDs, blockIDs)
	}
}

func validatePlanBlocksOnReplica(replica string, blocks ...string) validatorFunc {
	return func(t *testing.T, plan map[string]*blockPlanEntry) {
		planEntry, ok := plan[replica]
		require.True(t, ok, fmt.Sprintf("replica %s not found in plan", replica))
		for _, block := range blocks {
			require.Contains(t, planEntry.Ulids, block, "block %s not found in replica's %s plan", block, replica)
		}
	}
}

func Test_replicasPerBlockID_blockPlan(t *testing.T) {
	for _, tc := range []struct {
		name       string
		inputs     func(r *replicasPerBlockID)
		validators []validatorFunc
	}{
		{
			name: "single ingester",
			inputs: func(r *replicasPerBlockID) {
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
							newBlockInfo("b").info(),
							newBlockInfo("c").info(),
						},
					},
				}, ingesterInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("a", "b", "c"),
				validatePlanBlocksOnReplica("ingester-0", "a", "b", "c"),
			},
		},
		{
			name: "two ingester with duplicated blocks",
			inputs: func(r *replicasPerBlockID) {
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
							newBlockInfo("d").info(),
						},
					},
					{
						addr: "ingester-1",
						response: []*typesv1.BlockInfo{
							newBlockInfo("b").info(),
							newBlockInfo("d").info(),
						},
					},
				}, ingesterInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("a", "b", "d"),
				validatePlanBlocksOnReplica("ingester-0", "a", "d"),
				validatePlanBlocksOnReplica("ingester-1", "b"),
			},
		},
		{
			name: "prefer block on store-gateway over ingester",
			inputs: func(r *replicasPerBlockID) {
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
							newBlockInfo("b").info(),
						},
					},
				}, ingesterInstance)
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("a", "b"),
				validatePlanBlocksOnReplica("store-gateway-0", "a"),
				validatePlanBlocksOnReplica("ingester-0", "b"),
			},
		},
		{
			name: "ignore incomplete shards",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				t2, _ := time.Parse(time.RFC3339, "2021-01-01T01:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").withMinTime(t1, time.Hour-time.Second).info(),
							newBlockInfo("b").withMinTime(t2, time.Hour-time.Second).info(),
						},
					},
				}, ingesterInstance)
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a-1").
								withCompactionLevel(3).
								withCompactionSources("a").
								withCompactionParents("a").
								withCompactorShard(0, 2).
								withMinTime(t1, time.Hour-time.Second).
								info(),

							newBlockInfo("b-1").
								withCompactionLevel(3).
								withCompactionSources("b").
								withCompactionParents("b").
								withCompactorShard(0, 2).
								withMinTime(t2, time.Hour-(500*time.Millisecond)).info(),

							newBlockInfo("b-2").
								withCompactionLevel(3).
								withCompactionSources("b").
								withCompactionParents("b").
								withCompactorShard(1, 2).
								withMinTime(t2, time.Hour-time.Second).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("a", "b-1", "b-2"),
				validatePlanBlocksOnReplica("store-gateway-0", "b-1"),
				validatePlanBlocksOnReplica("store-gateway-0", "b-2"),
				validatePlanBlocksOnReplica("ingester-0", "a"),
			},
		},
		{
			// Using a split-and-merge compactor, deduplication happens at level 3,
			// level 2 is intermediate step, where series distributed among shards
			// but not yet deduplicated.
			name: "ignore blocks which are sharded and in level 2",
			inputs: func(r *replicasPerBlockID) {
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
							newBlockInfo("b").info(),
						},
					},
				}, ingesterInstance)
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("a").info(),
							newBlockInfo("a-1").
								withCompactionLevel(2).
								withCompactionSources("a").
								withCompactionParents("a").
								withCompactorShard(0, 2).
								info(),

							newBlockInfo("a-2").
								withCompactionLevel(2).
								withCompactionSources("a").
								withCompactionParents("a").
								withCompactorShard(1, 2).
								info(),

							newBlockInfo("a-3").
								withCompactionLevel(3).
								withCompactionSources("a-2").
								withCompactorShard(0, 3).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("a", "b"),
				validatePlanBlocksOnReplica("store-gateway-0", "a"),
				validatePlanBlocksOnReplica("ingester-0", "b"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReplicasPerBlockID(log.NewNopLogger())
			tc.inputs(r)

			plan := r.blockPlan(context.TODO())
			for _, v := range tc.validators {
				v(t, plan)
			}
		})
	}
}
