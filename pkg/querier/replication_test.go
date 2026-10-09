package querier

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/sharding"
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

// validatePlanDeduplication asserts the deduplication hint on every planned replica.
func validatePlanDeduplication(expected bool) validatorFunc {
	return func(t *testing.T, plan map[string]*blockPlanEntry) {
		require.NotEmpty(t, plan, "expected a non-empty plan to assert deduplication on")
		for replica, planEntry := range plan {
			require.Equal(t, expected, planEntry.Deduplication, "unexpected deduplication hint for replica %s", replica)
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
			// A window whose shards sit at different levels (shard 2 at L3, the rest
			// at L4, same minTime) is complete and must be queried, not pruned.
			name: "keep sharded window with shards at mixed compaction levels",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("s0").
								withCompactionLevel(4).
								withCompactorShard(0, 4).
								withMinTime(t1, 2*time.Hour).
								info(),
							newBlockInfo("s1").
								withCompactionLevel(4).
								withCompactorShard(1, 4).
								withMinTime(t1, 2*time.Hour).
								info(),
							// shard 2 lagged behind at level 3 (no late data touched it).
							newBlockInfo("s2").
								withCompactionLevel(3).
								withCompactorShard(2, 4).
								withMinTime(t1, time.Hour). // different maxTime, same minTime
								info(),
							newBlockInfo("s3").
								withCompactionLevel(4).
								withCompactorShard(3, 4).
								withMinTime(t1, 2*time.Hour).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// Pre-fix, (level, minTime) grouping split this into incomplete L3
				// and L4 groups and emptied the plan.
				validatePlanBlockIDs("s0", "s1", "s2", "s3"),
				// Complete deduplicated set -> served without query-time dedup.
				validatePlanDeduplication(false),
			},
		},
		{
			// Shard 0 has both its L3 and derived L4 present; shard 1 lags at L3.
			// Window is complete, and superseding collapses shard 0 to just its L4.
			name: "collapse L3+L4 duplicate of a shard within a mixed-level window",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							// shard 0: L4 derived from its L3 parent; both still present.
							newBlockInfo("s0-l4").
								withCompactionLevel(4).
								withCompactionSources("s0-l3").
								withCompactionParents("s0-l3").
								withCompactorShard(0, 2).
								withMinTime(t1, 2*time.Hour).
								info(),
							newBlockInfo("s0-l3").
								withCompactionLevel(3).
								withCompactorShard(0, 2).
								withMinTime(t1, time.Hour).
								info(),
							// shard 1: lagged at L3, no L4 derived yet.
							newBlockInfo("s1-l3").
								withCompactionLevel(3).
								withCompactorShard(1, 2).
								withMinTime(t1, time.Hour).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// One block per shard: shard 0's L3 superseded by its L4.
				validatePlanBlockIDs("s0-l4", "s1-l3"),
			},
		},
		{
			// Mixed spans: shard 0 merged to a wide [t1,t1+4h) block, while shard 1
			// still has two narrower blocks [t1,t1+2h) and [t1+2h,t1+4h). Completeness
			// must credit the wide block for the later window it covers, so shard 1's
			// [t1+2h,t1+4h) block is NOT orphaned and pruned. All three are kept.
			name: "keep mixed-span shards when a wide block covers a later window",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				t3 := t1.Add(2 * time.Hour)
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("s0-wide").withCompactionLevel(4).withCompactorShard(0, 2).withMinTime(t1, 4*time.Hour).info(),
							newBlockInfo("s1-a").withCompactionLevel(3).withCompactorShard(1, 2).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("s1-b").withCompactionLevel(3).withCompactorShard(1, 2).withMinTime(t3, 2*time.Hour).info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// Pre-fix, the (t1+2h) group saw only shard 1 and pruned s1-b, losing
				// shard 1's data for [t1+2h,t1+4h).
				validatePlanBlockIDs("s0-wide", "s1-a", "s1-b"),
				validatePlanDeduplication(false),
			},
		},
		{
			// A window with a genuinely missing shard (absent at every level) must
			// still be pruned: shard 2 of 4 has no block at any level.
			name: "prune sharded window with a shard missing at all levels",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "ingester-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("fallback").withMinTime(t1, 2*time.Hour).info(),
						},
					},
				}, ingesterInstance)
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("s0").
								withCompactionLevel(4).
								withCompactorShard(0, 4).
								withMinTime(t1, 2*time.Hour).
								info(),
							newBlockInfo("s1").
								withCompactionLevel(3).
								withCompactorShard(1, 4).
								withMinTime(t1, time.Hour).
								info(),
							// shard 2 is missing entirely; shard 3 too.
							newBlockInfo("s3").
								withCompactionLevel(4).
								withCompactorShard(3, 4).
								withMinTime(t1, 2*time.Hour).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// Incomplete across all levels -> sharded blocks pruned, fallback kept.
				validatePlanBlockIDs("fallback"),
				// Low-level fallback -> query-time dedup enabled.
				validatePlanDeduplication(true),
			},
		},
		{
			// Shard-count change (2 -> 4): a complete _of_2 and a partial _of_4
			// scheme share a level and minTime, so only shardCount separates them.
			// The _of_4 is pruned, the complete _of_2 served. Pre-fix, pooling by
			// {level, minTime} tripped the shard-length-mismatch guard and emptied
			// the plan.
			name: "serve complete old sharding when a shard-count change leaves a partial new one",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							// complete old 2-shard scheme
							newBlockInfo("old0").withCompactionLevel(4).withCompactorShard(0, 2).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("old1").withCompactionLevel(4).withCompactorShard(1, 2).withMinTime(t1, 2*time.Hour).info(),
							// partial new 4-shard scheme (shards 0 and 3 not yet produced)
							newBlockInfo("new1").withCompactionLevel(4).withCompactorShard(1, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("new2").withCompactionLevel(4).withCompactorShard(2, 4).withMinTime(t1, 2*time.Hour).info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs("old0", "old1"),
				validatePlanDeduplication(false),
			},
		},
		{
			// Two complete, unrelated shardings (_of_2 and _of_4) for the same
			// window: each is complete on its own so neither is pruned, and neither
			// supersedes the other. They partition the same series differently, so a
			// series can appear in both - query-time deduplication must be forced to
			// avoid double-counting.
			name: "deduplicate when two complete shardings overlap",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("of2-0").withCompactionLevel(4).withCompactorShard(0, 2).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of2-1").withCompactionLevel(4).withCompactorShard(1, 2).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-0").withCompactionLevel(4).withCompactorShard(0, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-1").withCompactionLevel(4).withCompactorShard(1, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-2").withCompactionLevel(4).withCompactorShard(2, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-3").withCompactionLevel(4).withCompactorShard(3, 4).withMinTime(t1, 2*time.Hour).info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// Both complete shardings survive; nothing is dropped.
				validatePlanBlockIDs("of2-0", "of2-1", "of4-0", "of4-1", "of4-2", "of4-3"),
				// Overlapping schemes -> query-time dedup forced.
				validatePlanDeduplication(true),
			},
		},
		{
			// Counterpart to the previous case: when the new _of_4 sharding is
			// derived from the old _of_2 one (lists it as sources), pruneSupersededBlocks
			// removes the superseded _of_2 blocks, leaving a single sharding. The
			// multi-sharding dedup guard must NOT fire here - the fast path
			// (Deduplication=false) is preserved.
			name: "single sharding after superseding keeps the fast path",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("of2-0").withCompactionLevel(3).withCompactorShard(0, 2).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of2-1").withCompactionLevel(3).withCompactorShard(1, 2).withMinTime(t1, 2*time.Hour).info(),
							// _of_4, one level up, derived from the _of_2 blocks.
							newBlockInfo("of4-0").withCompactionLevel(4).withCompactionSources("of2-0", "of2-1").withCompactorShard(0, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-1").withCompactionLevel(4).withCompactionSources("of2-0", "of2-1").withCompactorShard(1, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-2").withCompactionLevel(4).withCompactionSources("of2-0", "of2-1").withCompactorShard(2, 4).withMinTime(t1, 2*time.Hour).info(),
							newBlockInfo("of4-3").withCompactionLevel(4).withCompactionSources("of2-0", "of2-1").withCompactorShard(3, 4).withMinTime(t1, 2*time.Hour).info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				// _of_2 superseded by the derived _of_4; only one sharding remains.
				validatePlanBlockIDs("of4-0", "of4-1", "of4-2", "of4-3"),
				validatePlanDeduplication(false),
			},
		},
		{
			// Mid-merge: shard 0 at L3, shard 1 only in an intermediate L2 block, no
			// L1. The L2 block gets dropped by superseding, so it must not count shard
			// 1 as present - otherwise we'd serve shard 0 alone (half the data). The
			// set is incomplete -> everything pruned (transient empty, not undercount).
			name: "do not let an intermediate L2 block satisfy shard completeness",
			inputs: func(r *replicasPerBlockID) {
				t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")
				r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
					{
						addr: "store-gateway-0",
						response: []*typesv1.BlockInfo{
							newBlockInfo("s0-l3").
								withCompactionLevel(3).
								withCompactionSources("s0-l2").
								withCompactorShard(0, 2).
								withMinTime(t1, time.Hour).
								info(),
							newBlockInfo("s0-l2").
								withCompactionLevel(2).
								withCompactorShard(0, 2).
								withMinTime(t1, time.Hour).
								info(),
							// shard 1 only exists as an intermediate L2 block.
							newBlockInfo("s1-l2").
								withCompactionLevel(2).
								withCompactorShard(1, 2).
								withMinTime(t1, time.Hour).
								info(),
						},
					},
				}, storeGatewayInstance)
			},
			validators: []validatorFunc{
				validatePlanBlockIDs(),
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

// Pruning an incomplete window must emit a WARN (it was previously silent); a
// healthy mixed-level window must not.
func Test_pruneIncompleteShardedBlocks_logging(t *testing.T) {
	t1, _ := time.Parse(time.RFC3339, "2021-01-01T00:00:00Z")

	const warnMsg = "a shard is missing for this time window"

	t.Run("warns when pruning an incomplete sharded window", func(t *testing.T) {
		var buf bytes.Buffer
		r := newReplicasPerBlockID(log.NewLogfmtLogger(&buf))
		// shard 0 of 2 present, shard 1 missing at every level, no fallback.
		r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
			{
				addr: "store-gateway-0",
				response: []*typesv1.BlockInfo{
					newBlockInfo("s0").withCompactionLevel(3).withCompactorShard(0, 2).withMinTime(t1, 2*time.Hour).info(),
				},
			},
		}, storeGatewayInstance)

		plan := r.blockPlan(context.TODO())
		require.Empty(t, plan)
		require.Contains(t, buf.String(), warnMsg, "expected a WARN when an incomplete window is pruned")
	})

	t.Run("does not warn for a complete mixed-level window", func(t *testing.T) {
		var buf bytes.Buffer
		r := newReplicasPerBlockID(log.NewLogfmtLogger(&buf))
		// both shards present, at different levels.
		r.add([]ResponseFromReplica[[]*typesv1.BlockInfo]{
			{
				addr: "store-gateway-0",
				response: []*typesv1.BlockInfo{
					newBlockInfo("s0").withCompactionLevel(4).withCompactorShard(0, 2).withMinTime(t1, 2*time.Hour).info(),
					newBlockInfo("s1").withCompactionLevel(3).withCompactorShard(1, 2).withMinTime(t1, time.Hour).info(),
				},
			},
		}, storeGatewayInstance)

		plan := r.blockPlan(context.TODO())
		require.NotEmpty(t, plan)
		require.NotContains(t, buf.String(), warnMsg, "must not warn for a complete window")
	})
}
