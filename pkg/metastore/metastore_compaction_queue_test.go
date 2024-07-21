package metastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
)

func Test_compactionJobQueue(t *testing.T) {
	var now int64      // Timestamp of the raft command.
	lease := int64(10) // Job lease duration.
	q := newJobQueue(lease)

	assert.True(t, q.enqueue(&compactionpb.CompactionJob{
		Name:            "job1",
		RaftLogIndex:    1,
		CompactionLevel: 0,
	}))
	assert.True(t, q.enqueue(&compactionpb.CompactionJob{
		Name:            "job2",
		RaftLogIndex:    2,
		CompactionLevel: 1,
	}))
	assert.True(t, q.enqueue(&compactionpb.CompactionJob{
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

func assertJob(t *testing.T, j *compactionpb.CompactionJob, name string, commitIndex uint64) {
	require.NotNil(t, j)
	assert.Equal(t, name, j.Name)
	assert.Equal(t, commitIndex, j.RaftLogIndex)
}
