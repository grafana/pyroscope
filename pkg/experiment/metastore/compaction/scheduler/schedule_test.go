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
	"github.com/grafana/pyroscope/pkg/test/mocks/mockscheduler"
)

func TestSchedule_Update_LeaseRenewal(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	scheduler.queue.put(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 0,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		Token:           1,
		LeaseExpiresAt:  0,
	})

	t.Run("Owner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update := s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		assert.Equal(t, &raft_log.CompactionJobState{
			Name:            "1",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			Token:           1,
			LeaseExpiresAt:  int64(config.LeaseDuration),
		}, update)
	}))

	t.Run("NotOwner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		assert.Nil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		}))
	}))

	t.Run("JobCompleted", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		assert.Nil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "0",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		}))
	}))

	t.Run("WrongStatus", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		assert.Nil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
		}))
	}))
}

func TestSchedule_Update_JobCompleted(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	scheduler.queue.put(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 1,
		Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		Token:           1,
	})

	t.Run("Owner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		update := s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
		})
		assert.Equal(t, &raft_log.CompactionJobState{
			Name:            "1",
			CompactionLevel: 1,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			Token:           1,
		}, update)
	}))

	t.Run("NotOwner", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 0)})
		assert.Nil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{
			Name:   "1",
			Token:  0,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
		}))
	}))
}

func TestSchedule_Assign(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	// The job plans are accessed when it's getting assigned.
	// Their content is not important for the test.
	plans := []*raft_log.CompactionJobPlan{
		{Name: "2", CompactionLevel: 0},
		{Name: "3", CompactionLevel: 0},
		{Name: "1", CompactionLevel: 1},
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
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
		{Name: "4"},
		{Name: "5"},
		{Name: "6"},
	}
	for _, p := range plans {
		store.On("GetJobPlan", mock.Anything, p.Name).Return(p, nil)
	}

	now := int64(5)
	states := []*raft_log.CompactionJobState{
		// Jobs with expired leases (now > LeaseExpiresAt).
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 1},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 1},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 1},
		// This job can't be reassigned as its lease is still valid.
		{Name: "4", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 10},
		// The job has already failed in the past.
		{Name: "5", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 1, Failures: 1},
		// The job has already failed in the past and exceeded the error threshold.
		{Name: "6", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 1, Failures: 3},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	lease := now + int64(config.LeaseDuration)
	expected := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 2, LeaseExpiresAt: lease, Failures: 1},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 2, LeaseExpiresAt: lease, Failures: 1},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 2, LeaseExpiresAt: lease, Failures: 1},
		{Name: "5", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 2, LeaseExpiresAt: lease, Failures: 2},
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, now)})
		assigned := make([]*raft_log.CompactionJobState, 0, len(expected))
		for {
			update, err := s.AssignJob()
			require.NoError(t, err)
			if update == nil {
				break
			}
			assigned = append(assigned, update.State)
		}

		assert.Equal(t, expected, assigned)
	})
}

func TestSchedule_UpdateAssign(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
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
		updates := []*raft_log.CompactionJobStatusUpdate{
			{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
			{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
			{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1},
		}

		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		for i := range updates {
			update := s.UpdateJob(updates[i])
			assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, update.Status)
			assert.Equal(t, int64(updatedAt)+int64(config.LeaseDuration), update.LeaseExpiresAt)
			assert.Equal(t, uint64(1), update.Token) // Token must not change.
		}

		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
	})

	// If the worker reports success status and its lease has expired but the
	// job has not been reassigned, we accept the results.
	test.AssertIdempotent(t, func(t *testing.T) {
		updates := []*raft_log.CompactionJobStatusUpdate{
			{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1},
			{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1},
			{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, Token: 1},
		}

		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		for i := range updates {
			assert.NotNil(t, s.UpdateJob(updates[i]))
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
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 1)})
		for i := range plans {
			assert.Equal(t, states[i], s.AddJob(plans[i]))
		}
	})
}

func TestSchedule_QueueSizeLimit(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxQueueSize:  2,
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "1"},
		{Name: "2"},
		{Name: "3"},
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, AddedAt: 1, Token: 1},
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 1, AppendedAt: time.Unix(0, 1)})
		assert.Equal(t, states[0], s.AddJob(plans[0]))
		assert.Equal(t, states[1], s.AddJob(plans[1]))
		assert.Nil(t, s.AddJob(plans[2]))
	})
}

func TestSchedule_AssignEvict(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxQueueSize:  2,
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	plans := []*raft_log.CompactionJobPlan{
		{Name: "4"},
	}
	for _, p := range plans {
		store.On("GetJobPlan", mock.Anything, p.Name).Return(p, nil)
	}

	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "4", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 0},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		// Eviction is only possible when no jobs are available for assignment.
		assert.Nil(t, s.EvictJob())
		// Assign all the available jobs.
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Equal(t, "4", update.State.Name)
		update, err = s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
		// Now that no jobs can be assigned, we can try eviction.
		assert.NotNil(t, s.EvictJob())
		assert.NotNil(t, s.EvictJob())
		// MaxQueueSize reached.
		assert.Nil(t, s.EvictJob())
	})
}

func TestSchedule_Evict(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxQueueSize:  2,
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		// Eviction is only possible when no jobs are available for assignment.
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
		assert.NotNil(t, s.EvictJob())
		assert.Nil(t, s.EvictJob())
	})
}

func TestSchedule_NoEvict(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxQueueSize:  5,
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		// Eviction is only possible when no jobs are available for assignment.
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
		// Eviction is only possible when the queue size limit is reached.
		assert.Nil(t, s.EvictJob())
	})
}

func TestSchedule_NoEvictNoQueueSizeLimit(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	config := Config{
		MaxQueueSize:  0,
		MaxFailures:   3,
		LeaseDuration: 10 * time.Second,
	}

	scheduler := NewScheduler(config, store, nil)
	states := []*raft_log.CompactionJobState{
		{Name: "1", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "2", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
		{Name: "3", Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, Token: 1, LeaseExpiresAt: 0, Failures: 3},
	}
	for _, s := range states {
		scheduler.queue.put(s)
	}

	test.AssertIdempotent(t, func(t *testing.T) {
		updatedAt := time.Second * 20
		s := scheduler.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(0, int64(updatedAt))})
		// Eviction is only possible when no jobs are available for assignment.
		update, err := s.AssignJob()
		require.NoError(t, err)
		assert.Nil(t, update)
		// Eviction is not possible if the queue size limit is not set.
		assert.Nil(t, s.EvictJob())
	})
}
