package metastore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index/tombstones"
)

func TestRetryCompactionJob(t *testing.T) {
	ctx := context.Background()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "jobs.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	logger := log.NewNopLogger()
	ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
	c := compactor.NewCompactor(compactor.DefaultConfig(), compactor.NewStore(), ts, nil)
	store := scheduler.NewStore()
	sc := scheduler.NewScheduler(scheduler.Config{LeaseDuration: time.Minute, MaxFailures: 3}, store, nil)
	h := NewCompactionCommandHandler(logger, nil, c, c, sc, ts)
	r := &deletionTestRaft{db: db, handler: h, index: 10, now: time.Unix(100, 0)}
	svc := NewCompactionService(logger, r, deletionTestState{db: db}, h, sc, c)
	plan := &raft_log.CompactionJobPlan{Name: "job", Tenant: "tenant", Shard: 1, SourceBlocks: []string{"block"}}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, sc.Init(tx))
		require.NoError(t, c.Init(tx))
		require.NoError(t, ts.Init(tx))
		return sc.UpdateSchedule(tx, &raft_log.CompactionPlanUpdate{NewJobs: []*raft_log.NewCompactionJob{{
			Plan:  plan,
			State: &raft_log.CompactionJobState{Name: "job", Status: 1, Failures: 5, Token: 10, LeaseExpiresAt: 1, AddedAt: 1},
		}}})
	}))
	for _, req := range []*metastorev1.RetryCompactionJobRequest{nil, {}, {Name: "job"}, {Name: " ", Token: 10}} {
		_, err = svc.RetryCompactionJob(ctx, req)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	for _, req := range []*metastorev1.RetryCompactionJobRequest{{Name: "missing", Token: 10}, {Name: "job", Token: 9}} {
		_, err = svc.RetryCompactionJob(ctx, req)
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	}
	req := &metastorev1.RetryCompactionJobRequest{Name: "job", Token: 10}
	_, err = svc.RetryCompactionJob(ctx, req)
	require.NoError(t, err)
	_, err = svc.RetryCompactionJob(ctx, req)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a repeated click cannot grant another attempt")

	// Restore from persisted state before assignment, as on a restart.
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		got, e := store.GetJobPlan(tx, "job")
		require.NoError(t, e)
		require.Equal(t, plan, got)
		state, e := store.GetJobState(tx, "job")
		require.NoError(t, e)
		require.Equal(t, uint32(5), state.Failures)
		require.Equal(t, int64(1), state.AddedAt)
		require.Greater(t, state.Token, uint64(10))
		require.Zero(t, state.LeaseExpiresAt)
		require.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED, state.Status)
		return sc.Restore(tx)
	}))

	// Late reports cannot renew or complete the requeued job.
	for _, report := range []metastorev1.CompactionJobStatus{1, 2} {
		s := sc.NewSchedule(nil, &raft.Log{Index: r.index + 1, AppendedAt: r.now})
		require.Nil(t, s.UpdateJob(&raft_log.CompactionJobStatusUpdate{Name: "job", Token: 10, Status: report}))
	}
	poll, err := svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Len(t, poll.CompactionJobs, 1)
	require.Equal(t, plan.SourceBlocks, poll.CompactionJobs[0].SourceBlocks)
	assignment := poll.Assignments[0]
	_, err = svc.RetryCompactionJob(ctx, &metastorev1.RetryCompactionJobRequest{Name: "job", Token: assignment.Token})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "an active lease cannot be retried")
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		state, e := store.GetJobState(tx, "job")
		require.NoError(t, e)
		require.Equal(t, uint32(6), state.Failures)
		return nil
	}))

	// After the extra attempt expires, polling must not assign it again.
	r.now = r.now.Add(2 * time.Minute)
	poll, err = svc.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
	require.NoError(t, err)
	require.Empty(t, poll.CompactionJobs)
	_, err = svc.RetryCompactionJob(ctx, req)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "the original form stays stale even after the next failure")
	_, err = svc.RetryCompactionJob(ctx, &metastorev1.RetryCompactionJobRequest{Name: "job", Token: assignment.Token})
	require.NoError(t, err, "a fresh click can request the next attempt")
}

func TestRetryCompactionJob_LeaderChange(t *testing.T) {
	update := &raft_log.CompactionPlanUpdate{UpdatedJobs: []*raft_log.UpdatedCompactionJob{{State: &raft_log.CompactionJobState{Name: "job", Token: 2}}}}
	r := &proposalRecorder{responses: []proto.Message{
		&raft_log.GetCompactionPlanUpdateResponse{Term: 7, PlanUpdate: update},
		&raft_log.UpdateCompactionPlanResponse{},
	}}
	svc := NewCompactionService(log.NewNopLogger(), r, nil, nil, nil, nil)
	resp, err := svc.RetryCompactionJob(context.Background(), &metastorev1.RetryCompactionJobRequest{Name: "job", Token: 1})
	require.Nil(t, resp)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Len(t, r.proposals, 2)
	commit := r.proposals[1].(*raft_log.UpdateCompactionPlanRequest)
	require.Equal(t, uint64(7), commit.Term)
	require.Equal(t, update, commit.PlanUpdate)
}
