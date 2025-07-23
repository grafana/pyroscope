package scheduler

import (
	"container/heap"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

func TestJobQueue_order(t *testing.T) {
	items := []*raft_log.CompactionJobState{
		{
			Name:            "job-6",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			LeaseExpiresAt:  5,
			Failures:        0,
		},
		{
			Name:            "job-0",
			CompactionLevel: 1,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			LeaseExpiresAt:  2,
			Failures:        0,
		},
		{
			Name:            "job-1",
			CompactionLevel: 1,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			LeaseExpiresAt:  2,
			Failures:        0,
		},
		{
			Name:            "job-2",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt:  2,
			Failures:        0,
		},
		{
			Name:            "job-5",
			CompactionLevel: 1,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
			LeaseExpiresAt:  3,
			Failures:        0,
		},
		{
			Name:            "job-3",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt:  2,
			Failures:        5,
		},
		{
			Name:            "job-4",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt:  1,
			Failures:        5,
		},
	}

	test := func(items []*raft_log.CompactionJobState) {
		q := newJobQueue()
		for _, item := range items {
			q.put(item)
		}

		j3 := q.delete("job-1")
		j1 := q.delete("job-3")
		jx := q.delete("job-x")
		assert.Nil(t, jx)

		q.put(j1)
		q.put(j3)
		q.put(j3)
		q.put(j1)

		q.put(&raft_log.CompactionJobState{
			Name:            "job-4",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt:  3, // Should be after job-3.
			Failures:        5,
		})

		expected := []string{"job-6", "job-2", "job-3", "job-4", "job-0", "job-1", "job-5"}
		dequeued := make([]string, 0, len(items))
		for range items {
			x := jobQueuePop(q)
			assert.NotNil(t, x)
			dequeued = append(dequeued, x.Name)
		}
		assert.Equal(t, expected, dequeued)
		assert.Nil(t, jobQueuePop(q))
	}

	rnd := rand.New(rand.NewSource(123))
	for i := 0; i < 25; i++ {
		rnd.Shuffle(len(items), func(i, j int) {
			items[i], items[j] = items[j], items[i]
		})
		test(items)
	}
}

func TestJobQueue_delete(t *testing.T) {
	q := newJobQueue()
	items := []*raft_log.CompactionJobState{
		{Name: "job-1"},
		{Name: "job-2"},
		{Name: "job-3"},
		{Name: "job-4"},
	}

	for _, item := range items {
		q.put(item)
	}

	for _, item := range items {
		q.delete(item.Name)
	}

	assert.Nil(t, jobQueuePop(q))
}

func TestJobQueue_empty(t *testing.T) {
	q := newJobQueue()
	q.delete("job-1")
	assert.Nil(t, jobQueuePop(q))
	q.put(&raft_log.CompactionJobState{Name: "job-1"})
	q.delete("job-1")
	assert.Nil(t, jobQueuePop(q))
}

// The function is for testing purposes only.
func jobQueuePop(q *schedulerQueue) *raft_log.CompactionJobState {
	for i := range q.levels {
		level := q.level(uint32(i))
		if level.jobs.Len() > 0 {
			x := heap.Pop(level.jobs).(*jobEntry).CompactionJobState
			delete(q.jobs, x.Name)
			return x
		}
	}
	return nil
}
