package compactor

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockcompactor"
)

func TestSchedule_Update_LeaseRenewal(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := SchedulerConfig{
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

	s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
	t.Run("Owner", func(t *testing.T) {
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
	})

	t.Run("NotOwner", func(t *testing.T) {
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	})

	t.Run("JobCompleted", func(t *testing.T) {
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "0",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	})

	t.Run("WrongStatus", func(t *testing.T) {
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	})
}

func TestSchedule_Update_JobCompleted(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := SchedulerConfig{
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

	s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
	t.Run("Owner", func(t *testing.T) {
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks:    []string{"a", "b", "c"},
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
			SourceBlocks:    []string{"a", "b", "c"},
			CompactedBlocks: []*metastorev1.BlockMeta{{}},
		}, update.Plan)
	})

	t.Run("NotOwner", func(t *testing.T) {
		update, err := s.UpdateJob(&metastorev1.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks:    []string{"a", "b", "c"},
				CompactedBlocks: []*metastorev1.BlockMeta{{}},
			},
		})
		assert.NoError(t, err)
		assert.Nil(t, update)
	})
}

func TestSchedule_Assign(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := SchedulerConfig{
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

	s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
	for i := range plans {
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Equal(t, plans[i], update.Plan)
		assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.State.Status)
		assert.Equal(t, int64(config.LeaseDuration), update.State.LeaseExpiresAt)
		assert.Equal(t, uint64(1), update.State.Token)
	}

	update, err := s.AssignJob()
	require.NoError(t, err)
	assert.Nil(t, update)
}

func TestSchedule_ReAssign(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := SchedulerConfig{
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

	s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, 1)})
	for i := range plans {
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Equal(t, plans[i], update.Plan)
		assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.State.Status)
		assert.Equal(t, int64(config.LeaseDuration)+1, update.State.LeaseExpiresAt)
		assert.Equal(t, uint64(2), update.State.Token)
	}

	update, err := s.AssignJob()
	require.NoError(t, err)
	assert.Nil(t, update)
}

func TestSchedule_Add(t *testing.T) {
	store := new(mockcompactor.MockJobStore)
	config := SchedulerConfig{
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

	s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 1)})
	for i := range plans {
		update, err := s.AddJob(plans[i])
		require.NoError(t, err)
		assert.Equal(t, plans[i], update.Plan)
		assert.Equal(t, states[i], update.State)
	}
}
