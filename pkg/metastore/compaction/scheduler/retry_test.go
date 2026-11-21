package scheduler

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockscheduler"
)

func TestSchedule_RetryEligibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   metastorev1.CompactionJobStatus
		failures uint32
		limit    uint64
		lease    int64
		token    uint64
		allowed  bool
	}{
		{"exhausted", 1, 3, 3, 9, 1, true},
		{"above limit", 1, 5, 3, 9, 1, true},
		{"active lease", 1, 3, 3, 11, 1, false},
		{"lease boundary", 1, 3, 3, 10, 1, false},
		{"automatic retry available", 1, 2, 3, 9, 1, false},
		{"unlimited retries", 1, 3, 0, 9, 1, false},
		{"already queued", 0, 3, 3, 0, 1, false},
		{"completed", 2, 3, 3, 9, 1, false},
		{"stale token", 1, 3, 3, 9, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := NewScheduler(Config{MaxFailures: tc.limit}, nil, nil)
			state := &raft_log.CompactionJobState{Name: "job", Status: tc.status, Failures: tc.failures, LeaseExpiresAt: tc.lease, Token: 1, AddedAt: 4}
			original := state.CloneVT()
			sc.queue.put(state)
			s := sc.NewSchedule(nil, &raft.Log{Index: 3, AppendedAt: time.Unix(0, 10)})
			require.Nil(t, s.RetryJob(&metastorev1.RetryCompactionJobRequest{Name: "missing", Token: 1}))
			got := s.RetryJob(&metastorev1.RetryCompactionJobRequest{Name: "job", Token: tc.token})
			require.Equal(t, original, sc.queue.jobs["job"].CompactionJobState, "preparation must not mutate the queue")
			if !tc.allowed {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, uint64(3), got.Token)
			require.Equal(t, tc.failures, got.Failures)
			require.Equal(t, int64(4), got.AddedAt)
			require.Zero(t, got.LeaseExpiresAt)
			require.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, got.Status)
		})
	}
}

func TestSchedule_QueuedRetryNotEvicted(t *testing.T) {
	sc := NewScheduler(Config{MaxFailures: 3, MaxQueueSize: 1}, nil, nil)
	sc.queue.put(&raft_log.CompactionJobState{Name: "retry", Status: 0, Failures: math.MaxUint32})
	sc.queue.put(&raft_log.CompactionJobState{Name: "failed", Status: 1, Failures: 3})
	s := sc.NewSchedule(nil, &raft.Log{Index: 3, AppendedAt: time.Now()})
	require.Nil(t, s.EvictJob(), "an unassigned retry must not be evicted because of its retained failures")
}

func TestSchedule_RetryAssignment(t *testing.T) {
	for _, failures := range []uint32{3, math.MaxUint32} {
		t.Run(fmt.Sprint(failures), func(t *testing.T) {
			store := mockscheduler.NewMockJobStore(t)
			store.EXPECT().GetJobPlan(mock.Anything, "job").Return(&raft_log.CompactionJobPlan{Name: "job"}, nil).Once()
			sc := NewScheduler(Config{MaxFailures: 3, LeaseDuration: time.Second}, store, nil)
			sc.queue.put(&raft_log.CompactionJobState{Name: "job", Status: 1, Failures: failures, Token: 1})
			s := sc.NewSchedule(nil, &raft.Log{Index: 2, AppendedAt: time.Unix(1, 0)})
			requeued := s.RetryJob(&metastorev1.RetryCompactionJobRequest{Name: "job", Token: 1})
			require.NotNil(t, requeued)
			sc.queue.put(requeued)
			s = sc.NewSchedule(nil, &raft.Log{Index: 3, AppendedAt: time.Unix(1, 0)})
			assigned, err := s.AssignJob()
			require.NoError(t, err)
			require.NotNil(t, assigned)
			require.Equal(t, uint32(min(uint64(failures)+1, math.MaxUint32)), assigned.State.Failures)
			sc.queue.put(assigned.State)
			require.Equal(t, uint32(0), sc.queue.levels[0].stats.assignedTotal)
			require.Equal(t, uint32(1), sc.queue.levels[0].stats.reassignedTotal)
			s = sc.NewSchedule(nil, &raft.Log{Index: 4, AppendedAt: time.Unix(3, 0)})
			again, err := s.AssignJob()
			require.NoError(t, err)
			require.Nil(t, again)
			require.NotNil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{Name: "job", Token: 3, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS}), "the final worker can still finish")
		})
	}
}
