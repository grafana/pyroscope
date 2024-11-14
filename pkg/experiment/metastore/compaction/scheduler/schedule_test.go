package scheduler

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockcompactor"
)

func TestSchedule_Update_LeaseRenewal(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	scheduler.queue.put(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 0,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		Token:           1,
		LeaseExpiresAt:  0,
	})

	t.Run("Owner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.NoError(t, err)
		assert.Equal(t, &raft_log.CompactionJobState{
			Name:            "1",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			Token:           1,
			LeaseExpiresAt:  int64(config.LeaseDuration),
		}, update.State)
	}))

	t.Run("NotOwner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	}))

	t.Run("JobCompleted", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "0",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	}))

	t.Run("WrongStatus", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	}))
}

func TestSchedule_Update_JobCompleted(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	scheduler.queue.put(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 0,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		Token:           1,
		LeaseExpiresAt:  0,
	})
	store.On("GetJobPlan", mock.Anything, "1").
		Return(&raft_log.CompactionJobPlan{
			Name:            "1",
			Tenant:          "A",
			Shard:           1,
			CompactionLevel: 0,
			SourceBlocks:    []string{"a", "b", "c"},
		}, nil)

	t.Run("Owner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks:    &metastorev1.BlockList{Blocks: []string{"a", "b", "c"}},
				CompactedBlocks: []*metastorev1.BlockMeta{{}},
			},
		})
		assert.NoError(t, err)
		assert.Nil(t, update.State)
		assert.Equal(t, &raft_log.CompactionJobPlan{
			Name:            "1",
			Tenant:          "A",
			Shard:           1,
			CompactionLevel: 0,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks:    &metastorev1.BlockList{Blocks: []string{"a", "b", "c"}},
				CompactedBlocks: []*metastorev1.BlockMeta{{}},
			},
		}, update.Plan)
	}))

	t.Run("NotOwner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks:    &metastorev1.BlockList{Blocks: []string{"a", "b", "c"}},
				CompactedBlocks: []*metastorev1.BlockMeta{{}},
			},
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	}))
}

func TestSchedule_Assign(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "2", Tenant: "A", Shard: 1, CompactionLevel: 0, SourceBlocks: []string{"d", "e", "f"}},
		{Name: "3", Tenant: "A", Shard: 1, CompactionLevel: 0, SourceBlocks: []string{"j", "h", "i"}},
		{Name: "1", Tenant: "A", Shard: 1, CompactionLevel: 1, SourceBlocks: []string{"a", "b", "c"}},
	}
	for _, p := range plans {
		store.On("GetJobPlan", mock.Anything, p.Name).Return(p, nil)
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", CompactionLevel: 1, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED},
		{Name: "2", CompactionLevel: 0, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED},
		{Name: "3", CompactionLevel: 0, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED},
		{Name: "4", CompactionLevel: 0, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS},
		{Name: "5", CompactionLevel: 0, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		for j := range plans {
			update, err := s.AssignJob()
			require.NoError(t, err)
			assert.Equal(t, plans[j], update.Plan)
			assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.State.Status)
			assert.Equal(t, int64(config.LeaseDuration), update.State.LeaseExpiresAt)
			assert.Equal(t, uint64(1), update.State.Token)
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})
}

func TestSchedule_ReAssign(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1", Tenant: "A", Shard: 1, SourceBlocks: []string{"a", "b", "c"}},
		{Name: "2", Tenant: "A", Shard: 1, SourceBlocks: []string{"d", "e", "f"}},
		{Name: "3", Tenant: "A", Shard: 1, SourceBlocks: []string{"j", "h", "i"}},
	}
	for _, p := range plans {
		store.On("GetJobPlan", mock.Anything, p.Name).Return(p, nil)
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, 1)})
		for j := range plans {
			update, err := s.AssignJob()
			require.NoError(t, err)
			assert.Equal(t, plans[j], update.Plan)
			assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.State.Status)
			assert.Equal(t, int64(config.LeaseDuration)+1, update.State.LeaseExpiresAt)
			assert.Equal(t, uint64(2), update.State.Token)
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})
}

func TestSchedule_UpdateAssign(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1", Tenant: "A", Shard: 1, SourceBlocks: []string{"a", "b", "c"}},
		{Name: "2", Tenant: "A", Shard: 1, SourceBlocks: []string{"d", "e", "f"}},
		{Name: "3", Tenant: "A", Shard: 1, SourceBlocks: []string{"j", "h", "i"}},
	}
	for _, p := range plans {
		store.On("GetJobPlan", mock.Anything, p.Name).Return(p, nil)
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	// Lease is extended without reassignment if update arrives after the
	// expiration, but this is the first worker requested assignment.
	test.AssertIdempotent(t, func(t *testing.T) {
		updates := []*metastorev1.CompactionJobStatusUpdate{
			{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
			{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
			{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
		}

		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		for i := range updates {
			update, err := s.UpdateJob(updates[i])
			require.NoError(t, err)
			assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.State.Status)
			assert.Equal(t, int64(updatedAt)+int64(config.LeaseDuration), update.State.LeaseExpiresAt)
			assert.Equal(t, uint64(1), update.State.Token) // Token must not change.
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})

	// If the worker reports success status and its lease has expired but the
	// job has not been reassigned, we accept the results.
	test.AssertIdempotent(t, func(t *testing.T) {
		empty := &metastorev1.CompactedBlocks{}
		updates := []*metastorev1.CompactionJobStatusUpdate{
			{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1, CompactedBlocks: empty},
			{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1, CompactedBlocks: empty},
			{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1, CompactedBlocks: empty},
		}

		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		for i := range updates {
			update, err := s.UpdateJob(updates[i])
			require.NoError(t, err)
			assert.Nil(t, update.State) // Job is going to be deleted.
			assert.NotNil(t, update.Plan)
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})

	// The worker may be reassigned with the jobs it abandoned,
	// if it requested assignments first.
	test.AssertIdempotent(t, func(t *testing.T) {
		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		for range plans {
			update, err := s.AssignJob()
			require.NoError(t, err)
			assert.NotNil(t, update.State)
			assert.NotNil(t, update.Plan)
			assert.Equal(t, int64(updatedAt)+int64(config.LeaseDuration), update.State.LeaseExpiresAt)
			assert.Equal(t, uint64(2), update.State.Token) // Token must change.
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})
}

func TestSchedule_Add(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1", Tenant: "A", Shard: 1, SourceBlocks: []string{"a", "b", "c"}},
		{Name: "2", Tenant: "A", Shard: 1, SourceBlocks: []string{"d", "e", "f"}},
		{Name: "3", Tenant: "A", Shard: 1, SourceBlocks: []string{"j", "h", "i"}},
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 1)})
		for i := range plans {
			update, err := s.AddJob(plans[i])
			require.NoError(t, err)
			assert.Equal(t, plans[i], update.Plan)
			assert.Equal(t, states[i], update.State)
		}
	})
}
