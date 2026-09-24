package metastore

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"

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

func (fakePlanner) NewPlan(*raft.Log) compaction.Plan { return fakePlan{} }
func (fakePlanner) UpdatePlan(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error {
	return nil
}

type fakePlan struct{}

func (fakePlan) CreateJob() (*raft_log.CompactionJobPlan, error) { return nil, nil }

type fakeScheduler struct {
	compaction.Scheduler
	schedule compaction.Schedule
}

func (s fakeScheduler) NewSchedule(*bbolt.Tx, *raft.Log) compaction.Schedule {
	if s.schedule != nil {
		return s.schedule
	}
	return fakeSchedule{}
}

func (fakeScheduler) UpdateSchedule(*bbolt.Tx, *raft_log.CompactionPlanUpdate) error {
	return nil
}

type fakeSchedule struct {
	compaction.Schedule
	completed *raft_log.CompactionJobState
}

func (s fakeSchedule) UpdateJob(*raft_log.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	return s.completed
}

func (fakeSchedule) AssignJob() (*raft_log.AssignedCompactionJob, error) { return nil, nil }
func (fakeSchedule) EvictJob() *raft_log.CompactionJobState              { return nil }
func (fakeSchedule) AddJob(*raft_log.CompactionJobPlan) *raft_log.CompactionJobState {
	return nil
}

func TestCompactionCommandHandler_GetCompactionPlanUpdate_RejectOutput(t *testing.T) {
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
	compactedBlocks := &metastorev1.CompactedBlocks{
		SourceBlocks: &metastorev1.BlockList{
			Tenant: tenant,
			Shard:  shard,
			Blocks: []string{src1, src2},
		},
		NewBlocks: []*metastorev1.BlockMeta{blockMeta(out, 2)},
	}
	completed := &raft_log.CompactionJobState{
		Name:            "job-1",
		CompactionLevel: 1,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
	}

	type prepareSetup struct {
		handler *CompactionCommandHandler
		db      *bbolt.DB
	}

	newSetup := func(t *testing.T, stored ...string) prepareSetup {
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
		return prepareSetup{
			handler: NewCompactionCommandHandler(
				util.Logger,
				idx,
				new(fakeCompactor),
				fakePlanner{},
				fakeScheduler{schedule: fakeSchedule{completed: completed}},
				ts,
			),
			db: db,
		}
	}

	prepare := func(t *testing.T, s prepareSetup, status *raft_log.CompactionJobStatusUpdate) *raft_log.GetCompactionPlanUpdateResponse {
		cmd := &raft.Log{Index: 10, Term: 1, AppendedAt: time.Now()}
		req := &raft_log.GetCompactionPlanUpdateRequest{StatusUpdates: []*raft_log.CompactionJobStatusUpdate{status}}
		var resp *raft_log.GetCompactionPlanUpdateResponse
		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			var err error
			resp, err = s.handler.GetCompactionPlanUpdate(context.Background(), tx, cmd, req)
			return err
		}))
		return resp
	}

	t.Run("all source blocks exist", func(t *testing.T) {
		s := newSetup(t, src1, src2)
		resp := prepare(t, s, &raft_log.CompactionJobStatusUpdate{
			Name:            "job-1",
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: compactedBlocks,
		})
		require.Len(t, resp.PlanUpdate.CompletedJobs, 1)
		assert.False(t, resp.PlanUpdate.CompletedJobs[0].RejectOutput)
	})

	t.Run("some source blocks deleted", func(t *testing.T) {
		s := newSetup(t, src1) // src2 was removed, e.g., by retention.
		resp := prepare(t, s, &raft_log.CompactionJobStatusUpdate{
			Name:            "job-1",
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: compactedBlocks,
		})
		require.Len(t, resp.PlanUpdate.CompletedJobs, 1)
		assert.True(t, resp.PlanUpdate.CompletedJobs[0].RejectOutput)
	})

	t.Run("legacy reserved field compacted blocks", func(t *testing.T) {
		s := newSetup(t)
		legacy := &metastorev1.CompactionJobStatusUpdate{
			Name:            "job-1",
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: compactedBlocks,
		}
		raw, err := proto.Marshal(legacy)
		require.NoError(t, err)
		var status raft_log.CompactionJobStatusUpdate
		require.NoError(t, proto.Unmarshal(raw, &status))
		assert.Nil(t, status.GetCompactedBlocks())
		assert.NotEmpty(t, status.ProtoReflect().GetUnknown())

		resp := prepare(t, s, &status)
		require.Len(t, resp.PlanUpdate.CompletedJobs, 1)
		assert.True(t, resp.PlanUpdate.CompletedJobs[0].RejectOutput)
	})
}

func TestCompactionCommandHandler_UpdateCompactionPlan_RejectOutput(t *testing.T) {
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

	update := func(t *testing.T, s setup, job *raft_log.CompletedCompactionJob) {
		cmd := &raft.Log{Index: 10, Term: 1, AppendedAt: time.Now()}
		req := &raft_log.UpdateCompactionPlanRequest{
			Term: 1,
			PlanUpdate: &raft_log.CompactionPlanUpdate{
				CompletedJobs: []*raft_log.CompletedCompactionJob{job},
			},
		}
		require.NoError(t, s.db.Update(func(tx *bbolt.Tx) error {
			_, err := s.handler.UpdateCompactionPlan(context.Background(), tx, cmd, req)
			return err
		}))
	}

	acceptedJob := func() *raft_log.CompletedCompactionJob {
		return &raft_log.CompletedCompactionJob{
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
		}
	}

	sourceList := &metastorev1.BlockList{Tenant: tenant, Shard: shard, Blocks: []string{src1, src2}}
	outputList := &metastorev1.BlockList{Tenant: tenant, Shard: shard, Blocks: []string{out}}

	t.Run("accepted job replaces blocks", func(t *testing.T) {
		s := newSetup(t, src1, src2)
		update(t, s, acceptedJob())

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

	t.Run("reject_output skips replacement", func(t *testing.T) {
		s := newSetup(t, src1)
		job := acceptedJob()
		job.RejectOutput = true
		job.CompactedBlocks = nil
		update(t, s, job)

		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			assert.Equal(t, []string{out}, s.idx.MissingBlocks(tx, outputList))
			assert.Equal(t, []string{src2}, s.idx.MissingBlocks(tx, sourceList))
			return nil
		}))
		assert.Empty(t, s.compactor.compacted)
		assert.False(t, s.ts.Exists(tenant, shard, src1))
		assert.False(t, s.ts.Exists(tenant, shard, src2))
		assert.False(t, s.ts.Exists(tenant, shard, out))
	})

	t.Run("old leader plan is applied even if sources are missing", func(t *testing.T) {
		// Mixed-version compatibility: an old leader still proposes
		// CompactedBlocks without reject_output. New replicas must apply
		// that plan the same way old replicas do, not recompute rejection.
		s := newSetup(t, src1)
		update(t, s, acceptedJob())

		require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
			assert.Empty(t, s.idx.MissingBlocks(tx, outputList))
			// ReplaceBlocks deletes every listed source block that is still
			// present, so both IDs are missing afterwards.
			assert.Equal(t, []string{src1, src2}, s.idx.MissingBlocks(tx, sourceList))
			return nil
		}))
		assert.Equal(t, []string{out}, s.compactor.compacted)
	})
}

func TestCompactedBlocksFromStatus(t *testing.T) {
	want := &metastorev1.CompactedBlocks{
		SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Blocks: []string{"b1"}},
		NewBlocks:    []*metastorev1.BlockMeta{{Id: "b2"}},
	}

	t.Run("explicit field", func(t *testing.T) {
		got := compactedBlocksFromStatus(&raft_log.CompactionJobStatusUpdate{
			Name:            "job",
			CompactedBlocks: want,
		})
		assert.True(t, proto.Equal(want, got))
	})

	t.Run("reserved field fallback", func(t *testing.T) {
		legacy := &metastorev1.CompactionJobStatusUpdate{
			Name:            "job",
			CompactedBlocks: want,
		}
		raw, err := proto.Marshal(legacy)
		require.NoError(t, err)
		var status raft_log.CompactionJobStatusUpdate
		require.NoError(t, proto.Unmarshal(raw, &status))
		assert.Nil(t, status.GetCompactedBlocks())

		got := compactedBlocksFromStatus(&status)
		assert.True(t, proto.Equal(want, got))
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
