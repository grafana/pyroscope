package compaction

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

func TestScheduler_ownership(t *testing.T) {
	var now int64      // Timestamp of the raft command.
	lease := int64(10) // Job lease duration.
	q := newScheduler(lease)

	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "job1",
		RaftLogIndex:    1,
		CompactionLevel: 0,
	}))
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "job2",
		RaftLogIndex:    2,
		CompactionLevel: 1,
	}))
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "job3",
		RaftLogIndex:    3,
		CompactionLevel: 0,
	}))

	// Token here is the raft command index.
	assertJob(t, q.dequeue(now, 4), "job1", 4) // L0
	assertJob(t, q.dequeue(now, 5), "job3", 5) // L0
	assertJob(t, q.dequeue(now, 6), "job2", 6) // L1
	require.Nil(t, q.dequeue(now, 7))          // No jobs left.
	require.Nil(t, q.dequeue(now, 8))          // No jobs left.

	// Time has passed. Updating the jobs: all but job1.
	now += lease / 2
	assert.True(t, q.update("job3", now, 9))  // Postpone the deadline.
	assert.True(t, q.update("job2", now, 10)) // Postpone the deadline.
	require.Nil(t, q.dequeue(now, 11))        // No jobs left.

	// Time has passed: the initial lease has expired.
	now += lease/2 + 1
	assertJob(t, q.dequeue(now, 12), "job1", 12) // Seizing ownership of expired job.
	require.Nil(t, q.dequeue(now, 13))           // No jobs available yet.

	// Owner of the job1 awakes and tries to update the job.
	assert.False(t, q.update("job1", now, 4)) // Postpone the deadline; stale owner is rejected.
	assert.True(t, q.update("job1", now, 12)) // Postpone the deadline; new owner succeeds.

	assert.False(t, q.evict("job1", 4)) // Evicting the job; stale owner is rejected.
	assert.True(t, q.evict("job1", 12)) // Postpone the deadline; new owner succeeds.

	// Jobs are evicted in the end, regardless of the status.
	// We ignore expired lease, as long as nobody else has taken the job.
	assert.True(t, q.evict("job2", 10))
	assert.True(t, q.evict("job3", 9))

	// No jobs left.
	require.Nil(t, q.dequeue(now, 14))
}

func assertJob(t *testing.T, j *raft_log.CompactionJobState, name string, commitIndex uint64) {
	require.NotNil(t, j)
	assert.Equal(t, name, j.Name)
	assert.Equal(t, commitIndex, j.RaftLogIndex)
}

func TestScheduler_job_reassignment(t *testing.T) {
	var now int64      // Timestamp of the raft command.
	lease := int64(10) // Job lease duration.
	q := newScheduler(lease)

	jobs := []*raft_log.CompactionJobState{
		{CompactionLevel: 2},
		{CompactionLevel: 1},
		{CompactionLevel: 1},
		{CompactionLevel: 1},
		{CompactionLevel: 0},
		{CompactionLevel: 0},
		{CompactionLevel: 0},
		{CompactionLevel: 0},
	}
	for i, job := range jobs {
		job.RaftLogIndex = uint64(i + 1)
		job.Name = strconv.Itoa(len(jobs) - i)
		assert.True(t, q.enqueue(job))
	}

	raftIndex := uint64(len(jobs) + 1)
	now += 2 * lease // Not necessary as we have larger raft index.

	actual := make([]*raft_log.CompactionJobState, 0, len(jobs))
	for i := 0; i < len(jobs); i++ {
		actual = append(actual, q.dequeue(now, raftIndex))
		raftIndex++
	}

	expected := []*raft_log.CompactionJobState{
		{
			Name:            "1",
			CompactionLevel: 0,
			RaftLogIndex:    9,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "2",
			CompactionLevel: 0,
			RaftLogIndex:    10,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "3",
			CompactionLevel: 0,
			RaftLogIndex:    11,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "4",
			CompactionLevel: 0,
			RaftLogIndex:    12,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "5",
			CompactionLevel: 1,
			RaftLogIndex:    13,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "6",
			CompactionLevel: 1,
			RaftLogIndex:    14,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "7",
			CompactionLevel: 1,
			RaftLogIndex:    15,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
		{
			Name:            "8",
			CompactionLevel: 2,
			RaftLogIndex:    16,
			LeaseExpiresAt:  30,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		},
	}

	assert.Equal(t, expected, actual)
}

func TestScheduler_job_cancel(t *testing.T) {
	q := newScheduler(10)
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 0,
		RaftLogIndex:    1,
		LeaseExpiresAt:  30,
	}))
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "2",
		CompactionLevel: 0,
		RaftLogIndex:    2,
		LeaseExpiresAt:  30,
	}))

	assert.True(t, q.cancel("1"))

	// Lease expires.
	// Seizing ownership of expired job.
	assertJob(t, q.dequeue(10, 4), "2", 4)
	// Canceled job does not pop up.
	assert.Nil(t, q.dequeue(10, 5))
	assert.True(t, q.evict("2", 6))
	// Canceled job does not pop up.
	assert.Nil(t, q.dequeue(100, 7))
}

func TestScheduler_job_release(t *testing.T) {
	q := newScheduler(10)
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "1",
		CompactionLevel: 0,
		RaftLogIndex:    1,
		LeaseExpiresAt:  30,
	}))
	assert.True(t, q.enqueue(&raft_log.CompactionJobState{
		Name:            "2",
		CompactionLevel: 0,
		RaftLogIndex:    2,
		LeaseExpiresAt:  30,
	}))

	assertJob(t, q.dequeue(10, 3), "1", 3)
	assert.True(t, q.release("1"))

	assertJob(t, q.dequeue(10, 4), "1", 4)
	assertJob(t, q.dequeue(10, 5), "2", 5)
	assert.Nil(t, q.dequeue(10, 6))
}
