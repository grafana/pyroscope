package metastore

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
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

func Test_compactionJobQueue_from_snapshot(t *testing.T) {
	data, err := os.ReadFile("testdata/compaction_queue_snapshot.json")
	require.NoError(t, err)
	var jobs compactorv1.GetCompactionResponse
	err = protojson.Unmarshal(data, &jobs)
	require.NoError(t, err)

	lease := 15 * time.Second.Nanoseconds() // Job lease duration.
	q := newJobQueue(lease)

	for _, j := range jobs.CompactionJobs {
		e := q.enqueue(&compactionpb.CompactionJob{
			Name:              j.Name,
			Blocks:            j.Blocks,
			CompactionLevel:   j.CompactionLevel,
			RaftLogIndex:      j.RaftLogIndex,
			Shard:             j.Shard,
			TenantId:          j.TenantId,
			Status:            compactionpb.CompactionStatus(j.Status),
			LeaseExpiresAt:    j.LeaseExpiresAt,
			Failures:          j.Failures,
			LastFailureReason: j.LastFailureReason,
		})
		require.True(t, e)
	}

	for i := 0; i < q.pq.Len(); i++ {
		j := q.dequeue(time.Now().UnixNano(), math.MaxInt64)
		require.NotNilf(t, j, "nil element at %d", i)
	}
	j := q.dequeue(time.Now().UnixNano(), math.MaxInt64)
	require.Nil(t, j)
}

func assertJob(t *testing.T, j *compactionpb.CompactionJob, name string, commitIndex uint64) {
	require.NotNil(t, j)
	assert.Equal(t, name, j.Name)
	assert.Equal(t, commitIndex, j.RaftLogIndex)
}
