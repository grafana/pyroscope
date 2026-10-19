package metastore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index/tombstones"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
)

type deletionTestState struct {
	db  *bbolt.DB
	err error
}

func (s deletionTestState) ConsistentRead(_ context.Context, fn func(*bbolt.Tx, raftnode.ReadIndex)) error {
	if s.err != nil {
		return s.err
	}
	return s.db.View(func(tx *bbolt.Tx) error { fn(tx, raftnode.ReadIndex{Term: 1, CommitIndex: 1}); return nil })
}

type deletionTestRaft struct {
	db      *bbolt.DB
	handler *CompactionCommandHandler
	index   uint64
	now     time.Time
}

func (r *deletionTestRaft) Propose(ctx context.Context, _ fsm.RaftLogEntryType, req proto.Message) (resp proto.Message, err error) {
	r.index++
	now := r.now
	if now.IsZero() {
		now = time.Now()
	}
	cmd := &raft.Log{Index: r.index, Term: 1, AppendedAt: now}
	err = r.db.Update(func(tx *bbolt.Tx) error {
		var e error
		switch q := req.(type) {
		case *raft_log.GetCompactionPlanUpdateRequest:
			resp, e = r.handler.GetCompactionPlanUpdate(ctx, tx, cmd, q)
		case *raft_log.UpdateCompactionPlanRequest:
			resp, e = r.handler.UpdateCompactionPlan(ctx, tx, cmd, q)
		}
		return e
	})
	return resp, err
}

func TestDeleteDatasets_Validation(t *testing.T) {
	svc := NewCompactionService(log.NewNopLogger(), nil, nil, nil)
	for _, req := range []*metastorev1.DeleteDatasetsRequest{
		nil,
		{},
		{Tenant: "  "},
		{Tenant: "tenant-a"},
		{Tenant: "tenant-a", MinTime: 2, MaxTime: 1, Filter: &metastorev1.DeleteFilter{}},
		{Tenant: "tenant-a", Filter: &metastorev1.DeleteFilter{ServiceNameRegex: "["}},
	} {
		_, err := svc.DeleteDatasets(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestDeleteDatasets(t *testing.T) {
	ctx := context.Background()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "index.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	logger := log.NewNopLogger()
	idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
	ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
	cfg := compactor.DefaultConfig()
	queueStore := compactor.NewStore()
	c := compactor.NewCompactor(cfg, queueStore, ts, nil)
	sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 3}, scheduler.NewStore(), nil)
	h := NewCompactionCommandHandler(logger, idx, c, c, sc, ts)
	r := &deletionTestRaft{db: db, handler: h}
	svc := NewCompactionService(logger, r, deletionTestState{db: db}, h)
	// Metadata is milliseconds, while API requests and reports are nanoseconds.
	makeBlock := func(level uint32, owner int32, services ...int32) *metastorev1.BlockMeta {
		b := &metastorev1.BlockMeta{Id: ulid.Make().String(), Tenant: owner, Shard: 1, CompactionLevel: level, MinTime: 1000, MaxTime: 9000, StringTable: []string{"", "tenant-a", "tenant-b", "svc-a", "svc-b", "keep", "svc-ab"}}
		for _, name := range services {
			b.Datasets = append(b.Datasets, &metastorev1.Dataset{Tenant: 1, Name: name, MinTime: 2000, MaxTime: 8000, Size: 100})
		}
		return b
	}
	partial := makeBlock(2, 1, 3, 4, 5, 6)
	full := makeBlock(1, 1, 3)
	segment := makeBlock(0, 0, 3)
	segment.Datasets = append(segment.Datasets, &metastorev1.Dataset{Tenant: 2, Name: 3, MinTime: 1000, MaxTime: 9000, Size: 200})
	// A Format1 index must not count as a surviving real dataset.
	full.Datasets = append(full.Datasets, &metastorev1.Dataset{Format: 1, Tenant: 1, Size: 50})
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.Init(tx))
		require.NoError(t, ts.Init(tx))
		require.NoError(t, c.Init(tx))
		require.NoError(t, sc.Init(tx))
		for _, b := range []*metastorev1.BlockMeta{partial, full, segment} {
			require.NoError(t, idx.InsertBlock(tx, b.CloneVT()))
			require.NoError(t, c.Compact(tx, compaction.NewBlockEntry(&raft.Log{Index: 1, AppendedAt: time.Now()}, b)))
		}
		return nil
	}))
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		atBoundary, e := idx.DeletionCandidates(ctx, tx, "tenant-a", 9e9, 9e9)
		require.NoError(t, e)
		require.Len(t, atBoundary, 2, "only single-tenant blocks (CompactionLevel > 0 && Tenant != 0) are candidates")
		afterBoundary, e := idx.DeletionCandidates(ctx, tx, "tenant-a", 9e9+1, 10e9)
		require.NoError(t, e)
		require.Empty(t, afterBoundary)
		return nil
	}))
	// Tombstoned objects must never be scheduled for another rewrite, even
	// if their metadata is still present during cleanup or recovery.
	pending := makeBlock(1, 1, 3)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.InsertBlock(tx, pending.CloneVT()))
		return ts.AddTombstones(tx, &raft.Log{Index: 1, AppendedAt: time.Now()}, &metastorev1.Tombstones{Blocks: &metastorev1.BlockTombstones{
			Name: "already-pending", Tenant: "tenant-a", Shard: 1, CompactionLevel: 1, Blocks: []string{pending.Id},
		}})
	}))
	req := &metastorev1.DeleteDatasetsRequest{Tenant: "tenant-a", MinTime: 4e9, MaxTime: 5e9, Filter: &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a|svc-b"}, DryRun: true}
	resp, err := svc.DeleteDatasets(ctx, req)
	require.NoError(t, err)
	require.EqualValues(t, 2, resp.MatchedBlocksCount)
	require.EqualValues(t, 1, resp.PlannedFullBlockDeletionsCount)
	require.EqualValues(t, 1, resp.PlannedBlockRewritesCount)
	require.EqualValues(t, 3, resp.MatchedDatasetsCount)
	require.EqualValues(t, 300, resp.MatchedBytes)
	require.EqualValues(t, 2e9, resp.EffectiveMatchedMinTime)
	require.EqualValues(t, 8e9, resp.EffectiveMatchedMaxTime)
	require.Empty(t, resp.ScheduledJobNames)
	require.Zero(t, r.index, "dry run must not propose Raft commands")
	for _, s := range resp.Blocks {
		require.Positive(t, s.CompactionLevel)
		require.Equal(t, "tenant-a", s.Tenant)
		require.EqualValues(t, 1e9, s.BlockMinTime)
		require.EqualValues(t, 9e9, s.BlockMaxTime)
	}
	// Filtering by min_compaction_level excludes lower-level blocks (e.g., L1 full block).
	minLevelReq := req.CloneVT()
	minLevelReq.MinCompactionLevel = 2
	minLevelResp, err := svc.DeleteDatasets(ctx, minLevelReq)
	require.NoError(t, err)
	require.EqualValues(t, 1, minLevelResp.MatchedBlocksCount)
	require.Equal(t, partial.Id, minLevelResp.Blocks[0].BlockId)
	require.Equal(t, uint32(2), minLevelResp.Blocks[0].CompactionLevel)
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		jobs, e := sc.BusyBlocks(tx)
		require.NoError(t, e)
		require.Empty(t, jobs)
		return nil
	}))

	req.DryRun = false
	// Capacity rejection is represented in the Raft response, not a handler
	// error (which would cause the real FSM to panic). No partial jobs persist.
	h.scheduler = scheduler.NewScheduler(scheduler.Config{MaxQueueSize: 1}, scheduler.NewStore(), nil)
	_, err = svc.DeleteDatasets(ctx, req)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		jobs, e := sc.BusyBlocks(tx)
		require.NoError(t, e)
		require.Empty(t, jobs)
		return nil
	}))
	h.scheduler = sc
	var staleUpdate *raft_log.CompactionPlanUpdate
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		var e error
		_, staleUpdate, e = h.prepareDatasetDeletion(ctx, tx, &raft.Log{Index: 1, Term: 1, AppendedAt: time.Now()}, req)
		return e
	}))
	resp, err = svc.DeleteDatasets(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.ScheduledJobNames, 2)
	rejected, err := r.Propose(ctx, fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN), &raft_log.UpdateCompactionPlanRequest{Term: 1, PlanUpdate: staleUpdate})
	require.NoError(t, err)
	require.Nil(t, rejected.(*raft_log.UpdateCompactionPlanResponse).PlanUpdate, "stale plans must not race scheduled jobs")
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		it := queueStore.ListEntries(tx)
		defer func() { _ = it.Close() }()
		require.True(t, it.Next(), "anonymous L0 segment remains in the normal compaction queue")
		require.Zero(t, it.At().Level)
		require.False(t, it.Next(), "deletion sources must leave the normal compaction queue")
		return it.Err()
	}))
	_, err = svc.DeleteDatasets(ctx, req)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	// Persisted filters and deletion state survive scheduler restoration.
	require.NoError(t, db.View(func(tx *bbolt.Tx) error { return sc.Restore(tx) }))
	polled, err := svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 3})
	require.NoError(t, err)
	require.Len(t, polled.CompactionJobs, 2)
	updates := &metastorev1.PollCompactionJobsRequest{}
	for i, job := range polled.CompactionJobs {
		require.Equal(t, req.GetFilter().GetServiceNameRegex(), job.DeleteFilter.ServiceNameRegex)
		require.Equal(t, req.Tenant, job.Tenant)
		require.Positive(t, job.CompactionLevel)
		var replacements []*metastorev1.BlockMeta
		if job.SourceBlocks[0] != full.Id {
			b := partial.CloneVT()
			b.Id = ulid.Make().String()
			var keep []*metastorev1.Dataset
			for _, ds := range b.Datasets {
				if ds.Tenant != 1 || (ds.Name != 3 && ds.Name != 4) {
					keep = append(keep, ds)
				}
			}
			b.Datasets = keep
			replacements = []*metastorev1.BlockMeta{b}
		}
		updates.StatusUpdates = append(updates.StatusUpdates, &metastorev1.CompactionJobStatusUpdate{
			Name: job.Name, Token: polled.Assignments[i].Token, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{SourceBlocks: &metastorev1.BlockList{Tenant: job.Tenant, Shard: job.Shard, Blocks: job.SourceBlocks}, NewBlocks: replacements},
		})
	}
	_, err = svc.PollCompactionJobs(ctx, updates)
	require.NoError(t, err)
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		for _, b := range []*metastorev1.BlockMeta{partial, full} {
			owner := b.StringTable[b.Tenant]
			blocks, e := idx.GetBlocks(tx, &metastorev1.BlockList{Tenant: owner, Shard: b.Shard, Blocks: []string{b.Id}})
			require.NoError(t, e)
			require.Empty(t, blocks)
			require.True(t, ts.Exists(owner, b.Shard, b.Id))
		}
		// The anonymous L0 segment is untouched by dataset deletion.
		segBlocks, e := idx.GetBlocks(tx, &metastorev1.BlockList{Tenant: "", Shard: segment.Shard, Blocks: []string{segment.Id}})
		require.NoError(t, e)
		require.Len(t, segBlocks, 1)
		require.False(t, ts.Exists("", segment.Shard, segment.Id))

		jobs, e := sc.BusyBlocks(tx)
		require.NoError(t, e)
		require.Empty(t, jobs)
		remaining, e := idx.DeletionCandidates(ctx, tx, "tenant-a", 0, 10e9)
		require.NoError(t, e)
		require.Len(t, remaining, 2) // One replacement plus the pending tombstone.
		// The untouched L0 segment and the L1+ replacement (if below max level) are in the compaction queue.
		var queuedLevels []uint32
		it := queueStore.ListEntries(tx)
		defer func() { _ = it.Close() }()
		for it.Next() {
			queuedLevels = append(queuedLevels, it.At().Level)
		}
		require.Equal(t, []uint32{0, 2}, queuedLevels)
		return it.Err()
	}))
	resp, err = svc.DeleteDatasets(ctx, req)
	require.NoError(t, err)
	require.Zero(t, resp.MatchedBlocksCount)
}

func TestDeleteDatasets_CompactsToNextLevel(t *testing.T) {
	ctx := context.Background()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "index.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	logger := log.NewNopLogger()
	idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
	ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
	cfg := compactor.DefaultConfig()
	cfg.Levels = []compactor.LevelConfig{
		{MaxBlocks: 2}, // L0 -> L1
		{MaxBlocks: 2}, // L1 -> L2
		{MaxBlocks: 2}, // L2 -> L3 (final level: L3 >= len(Levels))
	}
	queueStore := compactor.NewStore()
	c := compactor.NewCompactor(cfg, queueStore, ts, nil)
	sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 3}, scheduler.NewStore(), nil)
	h := NewCompactionCommandHandler(logger, idx, c, c, sc, ts)
	r := &deletionTestRaft{db: db, handler: h}
	svc := NewCompactionService(logger, r, deletionTestState{db: db}, h)

	makeL1Block := func(services ...int32) *metastorev1.BlockMeta {
		b := &metastorev1.BlockMeta{
			Id:              ulid.Make().String(),
			Tenant:          1,
			Shard:           1,
			CompactionLevel: 1,
			MinTime:         1000,
			MaxTime:         9000,
			StringTable:     []string{"", "tenant-a", "svc-delete", "svc-keep"},
		}
		for _, name := range services {
			b.Datasets = append(b.Datasets, &metastorev1.Dataset{Tenant: 1, Name: name, MinTime: 2000, MaxTime: 8000, Size: 100})
		}
		return b
	}

	b1 := makeL1Block(2, 3) // has svc-delete and svc-keep
	b2 := makeL1Block(3)    // has only svc-keep

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.Init(tx))
		require.NoError(t, ts.Init(tx))
		require.NoError(t, c.Init(tx))
		require.NoError(t, sc.Init(tx))
		// Insert and enqueue b1 first (1 of 2 needed for L1->L2 compaction).
		require.NoError(t, idx.InsertBlock(tx, b1.CloneVT()))
		require.NoError(t, c.Compact(tx, compaction.NewBlockEntry(&raft.Log{Index: 1, AppendedAt: time.Now()}, b1)))
		return nil
	}))

	// 1. Schedule dataset deletion on L1 block b1.
	delReq := &metastorev1.DeleteDatasetsRequest{
		Tenant:  "tenant-a",
		MinTime: 1000 * 1e6,
		MaxTime: 9000 * 1e6,
		Filter:  &metastorev1.DeleteFilter{ServiceNameRegex: "svc-delete"},
	}
	delResp, err := svc.DeleteDatasets(ctx, delReq)
	require.NoError(t, err)
	require.Len(t, delResp.ScheduledJobNames, 1)

	// 2. Worker polls and receives the L1 deletion job.
	polled, err := svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Len(t, polled.CompactionJobs, 1)
	delJob := polled.CompactionJobs[0]
	require.NotNil(t, delJob.DeleteFilter)
	require.Equal(t, uint32(1), delJob.CompactionLevel)
	require.Equal(t, []string{b1.Id}, delJob.SourceBlocks)

	// 3. Worker completes the L1 deletion job, producing replacement L1 block b1Rewritten.
	b1Rewritten := makeL1Block(3)
	_, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
			Name:   delJob.Name,
			Token:  polled.Assignments[0].Token,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Shard: 1, Blocks: []string{b1.Id}},
				NewBlocks:    []*metastorev1.BlockMeta{b1Rewritten},
			},
		}},
	})
	require.NoError(t, err)

	// 4. Add the second L1 block b2 so L1 reaches MaxBlocks=2 (b1Rewritten + b2).
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.InsertBlock(tx, b2.CloneVT()))
		return c.Compact(tx, compaction.NewBlockEntry(&raft.Log{Index: 10, AppendedAt: time.Now()}, b2))
	}))

	// 5. Worker polls for normal compaction:
	// First poll plans and schedules the L1->L2 job; second poll assigns it to the worker.
	polled, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Empty(t, polled.CompactionJobs)
	polled, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Len(t, polled.CompactionJobs, 1)
	levelUpJob := polled.CompactionJobs[0]
	require.Nil(t, levelUpJob.DeleteFilter)
	require.Equal(t, uint32(1), levelUpJob.CompactionLevel)
	require.ElementsMatch(t, []string{b1Rewritten.Id, b2.Id}, levelUpJob.SourceBlocks)

	// 6. Worker completes the L1->L2 compaction job, producing an L2 block.
	bL2 := makeL1Block(3)
	bL2.CompactionLevel = 2
	_, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
			Name:   levelUpJob.Name,
			Token:  polled.Assignments[0].Token,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Shard: 1, Blocks: levelUpJob.SourceBlocks},
				NewBlocks:    []*metastorev1.BlockMeta{bL2},
			},
		}},
	})
	require.NoError(t, err)

	// Verify the L2 block is in the index, the L1 blocks were replaced and tombstoned,
	// and the L2 block is enqueued for L2->L3 compaction while a final-level L3 block is not.
	bL3 := makeL1Block(3)
	bL3.CompactionLevel = 3
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.InsertBlock(tx, bL3.CloneVT()))
		return c.Compact(tx, compaction.NewBlockEntry(&raft.Log{Index: 20, AppendedAt: time.Now()}, bL3))
	}))

	// Schedule and complete a deletion on the final-level (L3) block as well.
	bL3Delete := makeL1Block(2, 3)
	bL3Delete.CompactionLevel = 3
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return idx.InsertBlock(tx, bL3Delete.CloneVT())
	}))
	delL3Resp, err := svc.DeleteDatasets(ctx, delReq)
	require.NoError(t, err)
	require.Len(t, delL3Resp.ScheduledJobNames, 1)
	polledL3, err := svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Len(t, polledL3.CompactionJobs, 1)
	bL3Rewritten := makeL1Block(3)
	bL3Rewritten.CompactionLevel = 3
	_, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
			Name:   polledL3.CompactionJobs[0].Name,
			Token:  polledL3.Assignments[0].Token,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Shard: 1, Blocks: []string{bL3Delete.Id}},
				NewBlocks:    []*metastorev1.BlockMeta{bL3Rewritten},
			},
		}},
	})
	require.NoError(t, err)

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		oldBlocks, e := idx.GetBlocks(tx, &metastorev1.BlockList{Tenant: "tenant-a", Shard: 1, Blocks: []string{b1Rewritten.Id, b2.Id}})
		require.NoError(t, e)
		require.Empty(t, oldBlocks)
		require.True(t, ts.Exists("tenant-a", 1, b1Rewritten.Id))
		require.True(t, ts.Exists("tenant-a", 1, b2.Id))

		newBlocks, e := idx.GetBlocks(tx, &metastorev1.BlockList{Tenant: "tenant-a", Shard: 1, Blocks: []string{bL2.Id}})
		require.NoError(t, e)
		require.Len(t, newBlocks, 1)
		require.Equal(t, uint32(2), newBlocks[0].CompactionLevel)

		// Only the L2 block is in the compaction queue; the rewritten L3 (final level) block is not enqueued.
		it := queueStore.ListEntries(tx)
		defer func() { _ = it.Close() }()
		require.True(t, it.Next())
		require.Equal(t, bL2.Id, it.At().ID)
		require.Equal(t, uint32(2), it.At().Level)
		require.False(t, it.Next())
		return it.Err()
	}))
}

func TestDeleteDatasets_DryRunConcurrentTombstones(t *testing.T) {
	ctx := context.Background()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "index.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	logger := log.NewNopLogger()
	idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
	ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
	c := compactor.NewCompactor(compactor.DefaultConfig(), compactor.NewStore(), ts, nil)
	sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 3}, scheduler.NewStore(), nil)
	h := NewCompactionCommandHandler(logger, idx, c, c, sc, ts)
	svc := NewCompactionService(logger, &deletionTestRaft{db: db, handler: h}, deletionTestState{db: db}, h)

	b := &metastorev1.BlockMeta{
		Id:              ulid.Make().String(),
		Tenant:          1,
		Shard:           1,
		CompactionLevel: 1,
		MinTime:         1000,
		MaxTime:         9000,
		StringTable:     []string{"", "tenant-a", "svc-a"},
		Datasets:        []*metastorev1.Dataset{{Tenant: 1, Name: 2, MinTime: 2000, MaxTime: 8000, Size: 100}},
	}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, idx.Init(tx))
		require.NoError(t, ts.Init(tx))
		require.NoError(t, c.Init(tx))
		require.NoError(t, sc.Init(tx))
		return idx.InsertBlock(tx, b.CloneVT())
	}))

	req := &metastorev1.DeleteDatasetsRequest{
		Tenant:  "tenant-a",
		MinTime: 1000 * 1e6,
		MaxTime: 9000 * 1e6,
		Filter:  &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a"},
		DryRun:  true,
	}

	for i := 0; i < 100; i++ {
		ready := make(chan struct{}, 2)
		start := make(chan struct{})
		readResult := make(chan error, 1)
		writeResult := make(chan error, 1)
		tombstone := &metastorev1.Tombstones{
			Blocks: &metastorev1.BlockTombstones{
				Name:   ulid.Make().String(),
				Tenant: "tenant-a",
				Shard:  1,
				Blocks: []string{ulid.Make().String()},
			},
		}
		go func() {
			ready <- struct{}{}
			<-start
			_, err := svc.DeleteDatasets(ctx, req)
			readResult <- err
		}()
		go func() {
			ready <- struct{}{}
			<-start
			writeResult <- db.Update(func(tx *bbolt.Tx) error {
				cmd := &raft.Log{Index: uint64(i + 1), AppendedAt: time.Now()}
				if err := ts.AddTombstones(tx, cmd, tombstone); err != nil {
					return err
				}
				return ts.DeleteTombstones(tx, cmd, tombstone)
			})
		}()

		// Release both operations together on every iteration. This encourages
		// overlap without assuming a particular interleaving of their internals.
		<-ready
		<-ready
		close(start)
		// Join both operations before any fatal assertion can trigger cleanup.
		readErr, writeErr := <-readResult, <-writeResult
		require.NoError(t, readErr, "dry run at iteration %d", i)
		require.NoError(t, writeErr, "tombstone update at iteration %d", i)
	}
}

func TestCompactionService_ReadErrorHandling(t *testing.T) {
	ctx := context.Background()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "index.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	logger := log.NewNopLogger()
	sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 3}, scheduler.NewStore(), nil)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return sc.Init(tx)
	}))

	unavailableSvc := NewCompactionService(logger, nil, deletionTestState{err: errors.New("read index failed")}, nil)
	_, err = unavailableSvc.ListCompactionJobs(ctx, &metastorev1.ListCompactionJobsRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))

	_, err = unavailableSvc.DeleteDatasets(ctx, &metastorev1.DeleteDatasetsRequest{
		Tenant: "tenant-a",
		Filter: &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a"},
		DryRun: true,
	})
	require.Equal(t, codes.Unavailable, status.Code(err))

	// When a job state exists without its plan (or context is canceled), ListCompactionJobs returns a nil response on error.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return scheduler.NewStore().StoreJobState(tx, &raft_log.CompactionJobState{Name: "orphan-job"})
	}))
	svc := NewCompactionService(logger, nil, deletionTestState{db: db}, nil)
	listResp, err := svc.ListCompactionJobs(ctx, &metastorev1.ListCompactionJobsRequest{})
	require.Error(t, err)
	require.Nil(t, listResp)
}

func TestDeleteDatasets_FinalRetryRemainsBusy(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		t.Run(fmt.Sprintf("deletion=%t", deletion), func(t *testing.T) {
			ctx := context.Background()
			db, err := bbolt.Open(filepath.Join(t.TempDir(), "index.db"), 0600, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			logger := log.NewNopLogger()
			idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
			ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
			c := compactor.NewCompactor(compactor.DefaultConfig(), compactor.NewStore(), ts, nil)
			sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 1, MaxQueueSize: 1}, scheduler.NewStore(), nil)
			h := NewCompactionCommandHandler(logger, idx, c, c, sc, ts)
			r := &deletionTestRaft{db: db, handler: h}
			svc := NewCompactionService(logger, r, deletionTestState{db: db}, h)

			b := &metastorev1.BlockMeta{
				Id:              ulid.Make().String(),
				Tenant:          1,
				Shard:           1,
				CompactionLevel: 1,
				MinTime:         1000,
				MaxTime:         9000,
				StringTable:     []string{"", "tenant-a", "svc-a"},
				Datasets:        []*metastorev1.Dataset{{Tenant: 1, Name: 2, MinTime: 2000, MaxTime: 8000, Size: 100}},
			}
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				require.NoError(t, idx.Init(tx))
				require.NoError(t, ts.Init(tx))
				require.NoError(t, c.Init(tx))
				require.NoError(t, sc.Init(tx))
				return idx.InsertBlock(tx, b.CloneVT())
			}))

			req := &metastorev1.DeleteDatasetsRequest{
				Tenant:  "tenant-a",
				MinTime: 1000 * 1e6,
				MaxTime: 9000 * 1e6,
				Filter:  &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a"},
			}
			plan := &raft_log.CompactionJobPlan{
				Name: "existing-job", Tenant: req.Tenant, Shard: b.Shard,
				CompactionLevel: b.CompactionLevel, SourceBlocks: []string{b.Id},
			}
			if deletion {
				plan.DeleteFilter = req.Filter
			}
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				state := sc.NewSchedule(tx, &raft.Log{Index: 1, AppendedAt: time.Now()}).AddJob(plan)
				require.NotNil(t, state)
				return sc.UpdateSchedule(tx, &raft_log.CompactionPlanUpdate{
					NewJobs: []*raft_log.NewCompactionJob{{Plan: plan, State: state}},
				})
			}))

			r.now = time.Now()
			// The final retry reaches MaxFailures but still owns a valid lease.
			polled, err := svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
			require.NoError(t, err)
			require.Len(t, polled.CompactionJobs, 1)
			r.now = r.now.Add(2 * time.Minute)
			polled, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
			require.NoError(t, err)
			require.Len(t, polled.CompactionJobs, 1)
			finalAssignment := polled.Assignments[0]
			_, err = svc.DeleteDatasets(ctx, req)
			require.Equal(t, codes.FailedPrecondition, status.Code(err), "final retry still has a valid lease")
			polled, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
			require.NoError(t, err)
			require.Empty(t, polled.CompactionJobs, "job has exhausted MaxFailures and must not be assigned again")

			// Expiry and restoration do not fence off the last worker's completion.
			r.now = r.now.Add(2 * time.Minute)
			require.NoError(t, db.View(sc.Restore))
			_, err = svc.DeleteDatasets(ctx, req)
			require.Equal(t, codes.FailedPrecondition, status.Code(err))

			var replacements []*metastorev1.BlockMeta
			if !deletion {
				replacement := b.CloneVT()
				replacement.Id = ulid.Make().String()
				replacement.CompactionLevel++
				replacements = append(replacements, replacement)
			}
			_, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
				StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
					Name: plan.Name, Token: finalAssignment.Token,
					Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
					CompactedBlocks: &metastorev1.CompactedBlocks{
						SourceBlocks: &metastorev1.BlockList{Tenant: b.StringTable[b.Tenant], Shard: b.Shard, Blocks: []string{b.Id}},
						NewBlocks:    replacements,
					},
				}},
			})
			require.NoError(t, err)
			require.NoError(t, db.View(func(tx *bbolt.Tx) error {
				busy, err := sc.BusyBlocks(tx)
				require.NoError(t, err)
				require.Empty(t, busy)
				require.True(t, ts.Exists(req.Tenant, b.Shard, b.Id))
				return nil
			}))
			retryResp, err := svc.DeleteDatasets(ctx, req)
			require.NoError(t, err)
			if deletion {
				require.Zero(t, retryResp.MatchedBlocksCount)
			} else {
				require.Len(t, retryResp.ScheduledJobNames, 1, "replacement is available for deletion only after compaction completes")
			}
		})
	}
}
