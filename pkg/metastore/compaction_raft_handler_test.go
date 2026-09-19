package metastore

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index/tombstones"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type fakeCompactor struct{ compacted []string }

func (c *fakeCompactor) Compact(_ *bbolt.Tx, e compaction.BlockEntry) error {
	c.compacted = append(c.compacted, e.ID)
	return nil
}

// The embedded interfaces are nil: calls to methods that are not
// overridden panic, which makes unexpected calls visible in the test.
type fakePlanner struct{ compaction.Planner }

func (fakePlanner) UpdatePlan(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error { return nil }

type fakeScheduler struct{ compaction.Scheduler }

func (fakeScheduler) UpdateSchedule(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error { return nil }

func TestCompactionCommandHandler_UpdateCompactionPlan_SourceBlocksDeleted(t *testing.T) {
	const (
		tenant = "tenant-a"
		shard  = uint32(1)
	)

	blockMeta := func(id string, level uint32) *metastorev1.BlockMeta {
		return &metastorev1.BlockMeta{
			Id:              id,
			Tenant:          1,
			Shard:           shard,
			CompactionLevel: level,
			MinTime:         test.UnixMilli("2024-09-11T07:00:00.000Z"),
			MaxTime:         test.UnixMilli("2024-09-11T08:00:00.000Z"),
			CreatedBy:       2,
			StringTable:     []string{"", tenant, "compaction-worker"},
		}
	}

	src1 := test.ULID("2024-09-11T07:00:00.001Z")
	src2 := test.ULID("2024-09-11T07:00:00.002Z")
	out := test.ULID("2024-09-11T07:00:00.003Z")

	type setup struct {
		handler   *CompactionCommandHandler
		idx       *index.Index
		ts        *tombstones.Tombstones
		compactor *fakeCompactor
		db        *bbolt.DB
	}

	newSetup := func(t *testing.T, stored ...string) setup {
		db := test.BoltDB(t)
		t.Cleanup(func() { _ = db.Close() })
		idx := index.NewIndex(util.Logger, index.NewStore(), index.DefaultConfig, nil)
		ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
		require.NoError(t, db.Update(idx.Init))
		require.NoError(t, db.Update(ts.Init))
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			for _, id := range stored {
				if err := idx.InsertBlock(tx, blockMeta(id, 1)); err != nil {
					return err
				}
			}
			return nil
		}))
		c := new(fakeCompactor)
		h := NewCompactionCommandHandler(util.Logger, idx, c, fakePlanner{}, fakeScheduler{}, ts)
		return setup{handler: h, idx: idx, ts: ts, compactor: c, db: db}
	}

	update := func(t *testing.T, s setup) {
		cmd := &raft.Log{Index: 10, Term: 1, AppendedAt: time.Now()}
		req := &raft_log.UpdateCompactionPlanRequest{
			Term: 1,
			PlanUpdate: &raft_log.CompactionPlanUpdate{
				CompletedJobs: []*raft_log.CompletedCompactionJob{{
					State: &raft_log.CompactionJobState{
						Name:            "job-1",
						CompactionLevel: 1,
						Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
					},
					CompactedBlocks: &metastorev1.CompactedBlocks{
						SourceBlocks: &metastorev1.BlockList{
							Tenant: tenant,
							Shard:  shard,
							Blocks: []string{src1, src2},
						},
						NewBlocks: []*metastorev1.BlockMeta{blockMeta(out, 2)},
					},
				}},
			},
		}
		require.NoError(t, s.db.Update(func(tx *bbolt.Tx) error {
			_, err := s.handler.UpdateCompactionPlan(context.Background(), tx, cmd, req)
			return err
		}))
	}

	sourceList := &metastorev1.BlockList{Tenant: tenant, Shard: shard, Blocks: []string{src1, src2}}
	outputList := &metastorev1.BlockList{Tenant: tenant, Shard: shard, Blocks: []string{out}}

	t.Run("all source blocks exist", func(t *testing.T) {
		s := newSetup(t, src1, src2)
		update(t, s)

		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			assert.Equal(t, []string{src1, src2}, s.idx.MissingBlocks(tx, sourceList))
			assert.Empty(t, s.idx.MissingBlocks(tx, outputList))
			return nil
		}))
		assert.Equal(t, []string{out}, s.compactor.compacted)
		assert.True(t, s.ts.Exists(tenant, shard, src1))
		assert.True(t, s.ts.Exists(tenant, shard, src2))
		assert.False(t, s.ts.Exists(tenant, shard, out))
	})

	t.Run("some source blocks deleted", func(t *testing.T) {
		s := newSetup(t, src1) // src2 was removed, e.g., by retention.
		update(t, s)

		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			// The output block is not added, and the remaining source
			// block stays in the index.
			assert.Equal(t, []string{out}, s.idx.MissingBlocks(tx, outputList))
			assert.Equal(t, []string{src2}, s.idx.MissingBlocks(tx, sourceList))
			return nil
		}))
		assert.Empty(t, s.compactor.compacted)
		assert.True(t, s.ts.Exists(tenant, shard, out))
		assert.False(t, s.ts.Exists(tenant, shard, src1))
		assert.False(t, s.ts.Exists(tenant, shard, src2))
	})

	t.Run("all source blocks deleted", func(t *testing.T) {
		s := newSetup(t)
		update(t, s)

		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			assert.Equal(t, []string{out}, s.idx.MissingBlocks(tx, outputList))
			return nil
		}))
		assert.Empty(t, s.compactor.compacted)
		assert.True(t, s.ts.Exists(tenant, shard, out))
	})
}

func TestTombstonesForRejectedBlocks(t *testing.T) {
	md := func(id, tenant string, shard, level uint32) *metastorev1.BlockMeta {
		return &metastorev1.BlockMeta{
			Id:              id,
			Tenant:          1,
			Shard:           shard,
			CompactionLevel: level,
			StringTable:     []string{"", tenant},
		}
	}

	actual := tombstonesForRejectedBlocks("job", []*metastorev1.BlockMeta{
		md("a", "t1", 1, 1),
		md("b", "t2", 1, 1),
		md("c", "t1", 1, 1),
	})

	expected := []*metastorev1.Tombstones{
		{Blocks: &metastorev1.BlockTombstones{
			Name: "job-rejected-0", Tenant: "t1", Shard: 1, CompactionLevel: 1,
			Blocks: []string{"a", "c"},
		}},
		{Blocks: &metastorev1.BlockTombstones{
			Name: "job-rejected-1", Tenant: "t2", Shard: 1, CompactionLevel: 1,
			Blocks: []string{"b"},
		}},
	}

	require.Len(t, actual, len(expected))
	for i := range expected {
		assert.True(t, expected[i].EqualVT(actual[i]), "tombstone %d: %v", i, actual[i])
	}
}
