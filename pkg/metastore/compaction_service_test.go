package metastore

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

// localReadState reads the local database directly, bypassing raft:
// a State implementation for tests.
type localReadState struct{ db *bbolt.DB }

func (s localReadState) ConsistentRead(_ context.Context, read func(*bbolt.Tx, raftnode.ReadIndex)) error {
	return s.db.View(func(tx *bbolt.Tx) error {
		read(tx, raftnode.ReadIndex{})
		return nil
	})
}

func TestCompactionService_GetCompactionState(t *testing.T) {
	db := test.BoltDB(t)

	jobStore := scheduler.NewStore()
	blockStore := compactor.NewStore()
	schedulerConfig := scheduler.Config{
		LeaseDuration: 15 * time.Second,
		MaxFailures:   3,
		MaxQueueSize:  100,
	}
	sched := scheduler.NewScheduler(schedulerConfig, jobStore, nil)
	comp := compactor.NewCompactor(compactor.DefaultConfig(), blockStore, nil, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, jobStore.CreateBuckets(tx))
		require.NoError(t, blockStore.CreateBuckets(tx))
		require.NoError(t, jobStore.StoreJobState(tx, &raft_log.CompactionJobState{
			Name:            "job-a",
			CompactionLevel: 0,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			Token:           42,
			LeaseExpiresAt:  time.Unix(0, 0).Add(time.Hour).UnixNano(),
			AddedAt:         time.Unix(0, 0).UnixNano(),
		}))
		require.NoError(t, jobStore.StoreJobPlan(tx, &raft_log.CompactionJobPlan{
			Name:            "job-a",
			Tenant:          "tenant-a",
			Shard:           1,
			CompactionLevel: 0,
			SourceBlocks:    []string{"b1", "b2", "b3"},
		}))
		require.NoError(t, jobStore.StoreJobState(tx, &raft_log.CompactionJobState{
			Name:            "job-b",
			CompactionLevel: 1,
		}))
		require.NoError(t, blockStore.StoreEntry(tx, compaction.BlockEntry{
			Index: 1, ID: "b4", Tenant: "tenant-a", Shard: 1, Level: 0, AppendedAt: 100,
		}))
		require.NoError(t, blockStore.StoreEntry(tx, compaction.BlockEntry{
			Index: 2, ID: "b5", Tenant: "tenant-a", Shard: 1, Level: 0, AppendedAt: 200,
		}))
		return nil
	}))

	svc := NewCompactionService(log.NewNopLogger(), nil, localReadState{db}, sched, comp)

	resp, err := svc.GetCompactionState(context.Background(), new(metastorev1.GetCompactionStateRequest))
	require.NoError(t, err)

	assert.Equal(t, (15 * time.Second).Nanoseconds(), resp.JobLeaseDuration)
	assert.Equal(t, uint64(3), resp.JobMaxFailures)
	assert.Equal(t, uint64(100), resp.MaxJobQueueSize)

	require.Len(t, resp.CompactionJobs, 2)
	jobA := resp.CompactionJobs[0]
	assert.Equal(t, "job-a", jobA.Name)
	assert.Equal(t, "tenant-a", jobA.Tenant)
	assert.Equal(t, uint32(1), jobA.Shard)
	assert.Equal(t, uint32(3), jobA.SourceBlocks)
	assert.Equal(t, uint64(42), jobA.Token)
	assert.Empty(t, jobA.WorkerId)
	jobB := resp.CompactionJobs[1]
	assert.Equal(t, "job-b", jobB.Name)
	assert.Empty(t, jobB.Tenant)

	require.Len(t, resp.CompactionQueues, 1)
	queue := resp.CompactionQueues[0]
	assert.Equal(t, "tenant-a", queue.Tenant)
	assert.Equal(t, uint32(1), queue.Shard)
	assert.Equal(t, uint64(2), queue.Blocks)
	assert.Equal(t, int64(100), queue.OldestBlockAt)
	assert.Equal(t, int64(200), queue.NewestBlockAt)

	// The worker attribution is reported once the job assignment
	// is observed, and the fencing tokens match.
	svc.updateOwners("worker-1", &raft_log.CompactionPlanUpdate{
		AssignedJobs: []*raft_log.AssignedCompactionJob{
			{State: &raft_log.CompactionJobState{Name: "job-a", Token: 42}},
		},
	})
	resp, err = svc.GetCompactionState(context.Background(), new(metastorev1.GetCompactionStateRequest))
	require.NoError(t, err)
	jobA = resp.CompactionJobs[0]
	assert.Equal(t, "worker-1", jobA.WorkerId)
	assert.NotZero(t, jobA.AssignedAt)
	assert.NotZero(t, jobA.UpdatedAt)

	// Attribution with a mismatching token is not reported: the observed
	// assignment does not refer to the current state of the job.
	svc.updateOwners("worker-2", &raft_log.CompactionPlanUpdate{
		AssignedJobs: []*raft_log.AssignedCompactionJob{
			{State: &raft_log.CompactionJobState{Name: "job-a", Token: 43}},
		},
	})
	resp, err = svc.GetCompactionState(context.Background(), new(metastorev1.GetCompactionStateRequest))
	require.NoError(t, err)
	assert.Empty(t, resp.CompactionJobs[0].WorkerId)

	// A lease renewal of a not-yet-observed job restores the attribution;
	// the assignment time remains unknown.
	svc.updateOwners("worker-3", &raft_log.CompactionPlanUpdate{
		UpdatedJobs: []*raft_log.UpdatedCompactionJob{
			{State: &raft_log.CompactionJobState{Name: "job-a", Token: 42}},
		},
	})
	resp, err = svc.GetCompactionState(context.Background(), new(metastorev1.GetCompactionStateRequest))
	require.NoError(t, err)
	jobA = resp.CompactionJobs[0]
	assert.Equal(t, "worker-3", jobA.WorkerId)
	assert.Zero(t, jobA.AssignedAt)
	assert.NotZero(t, jobA.UpdatedAt)

	// Completion removes the attribution.
	svc.updateOwners("worker-3", &raft_log.CompactionPlanUpdate{
		CompletedJobs: []*raft_log.CompletedCompactionJob{
			{State: &raft_log.CompactionJobState{Name: "job-a"}},
		},
	})
	svc.ownersMu.Lock()
	assert.Empty(t, svc.owners)
	svc.ownersMu.Unlock()
}

// proposalRecorder is a Raft implementation that records the proposed
// commands and returns canned responses.
type proposalRecorder struct {
	proposals []proto.Message
	responses []proto.Message
}

func (r *proposalRecorder) Propose(_ context.Context, _ fsm.RaftLogEntryType, m proto.Message) (proto.Message, error) {
	r.proposals = append(r.proposals, m)
	resp := r.responses[0]
	r.responses = r.responses[1:]
	return resp, nil
}

// The test pins the content of the raft proposals: the prepare step must
// propose the raft_log request stripped of the compacted blocks (these are
// only replicated with the final plan update), and must not include the
// worker identity.
func TestCompactionService_PollCompactionJobs_proposals(t *testing.T) {
	completed := &raft_log.CompactionJobState{
		Name:   "job-done",
		Token:  40,
		Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
	}
	assigned := &raft_log.CompactionJobState{
		Name:   "job-new",
		Token:  41,
		Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
	}
	planUpdate := &raft_log.CompactionPlanUpdate{
		CompletedJobs: []*raft_log.CompletedCompactionJob{{State: completed}},
		AssignedJobs: []*raft_log.AssignedCompactionJob{{
			State: assigned,
			Plan:  &raft_log.CompactionJobPlan{Name: "job-new", Tenant: "tenant-a"},
		}},
	}
	raft := &proposalRecorder{responses: []proto.Message{
		&raft_log.GetCompactionPlanUpdateResponse{Term: 1, PlanUpdate: planUpdate},
		&raft_log.UpdateCompactionPlanResponse{PlanUpdate: planUpdate},
	}}

	svc := NewCompactionService(log.NewNopLogger(), raft, nil, nil, nil)
	compactedBlocks := &metastorev1.CompactedBlocks{
		SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Blocks: []string{"b1"}},
		NewBlocks:    []*metastorev1.BlockMeta{{Id: "b2"}},
	}
	resp, err := svc.PollCompactionJobs(context.Background(), &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
			Name:            "job-done",
			Token:           40,
			Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			CompactedBlocks: compactedBlocks,
		}},
		JobCapacity: 1,
		WorkerId:    "worker-1",
	})
	require.NoError(t, err)
	require.Len(t, raft.proposals, 2)

	prepare, ok := raft.proposals[0].(*raft_log.GetCompactionPlanUpdateRequest)
	require.True(t, ok, "the prepare step must propose the raft_log request, got %T", raft.proposals[0])
	assert.Equal(t, uint32(1), prepare.AssignJobsMax)
	require.Len(t, prepare.StatusUpdates, 1)
	assert.Equal(t, "job-done", prepare.StatusUpdates[0].Name)
	assert.Equal(t, uint64(40), prepare.StatusUpdates[0].Token)

	proposal, ok := raft.proposals[1].(*raft_log.UpdateCompactionPlanRequest)
	require.True(t, ok, "the update step must propose the raft_log request, got %T", raft.proposals[1])
	assert.Equal(t, uint64(1), proposal.Term)
	// The compacted blocks reported by the worker are attached
	// to the completed job of the final proposal.
	require.Len(t, proposal.PlanUpdate.CompletedJobs, 1)
	assert.Equal(t, compactedBlocks, proposal.PlanUpdate.CompletedJobs[0].CompactedBlocks)

	// The worker response includes the new assignment and its plan.
	require.Len(t, resp.CompactionJobs, 1)
	assert.Equal(t, "job-new", resp.CompactionJobs[0].Name)
	require.Len(t, resp.Assignments, 1)
	assert.Equal(t, uint64(41), resp.Assignments[0].Token)

	// The assignment attribution is recorded for the calling worker.
	svc.ownersMu.Lock()
	require.Contains(t, svc.owners, "job-new")
	assert.Equal(t, "worker-1", svc.owners["job-new"].worker)
	assert.NotContains(t, svc.owners, "job-done")
	svc.ownersMu.Unlock()
}

func TestCompactionService_workerID(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "worker-1", workerID(ctx, &metastorev1.PollCompactionJobsRequest{WorkerId: "worker-1"}))
	assert.Equal(t, "", workerID(ctx, new(metastorev1.PollCompactionJobsRequest)))

	addr := &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 4242}
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: addr})
	assert.Equal(t, addr.String(), workerID(ctx, new(metastorev1.PollCompactionJobsRequest)))
	assert.Equal(t, "worker-1", workerID(ctx, &metastorev1.PollCompactionJobsRequest{WorkerId: "worker-1"}))
}

// Prior to https://github.com/grafana/pyroscope/pull/5465, the prepare step
// proposed the worker request as-is, so the raft log may contain
// metastore.v1.PollCompactionJobsRequest entries under the
// GET_COMPACTION_PLAN_UPDATE command. The FSM decodes the entry payload as
// raft_log.GetCompactionPlanUpdateRequest: replaying such a log must not fail,
// and the entry must be interpreted exactly as it was when written.
func TestGetCompactionPlanUpdateRequest_oldLogEntry(t *testing.T) {
	oldEntry := &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
			Name:   "job-done",
			Token:  40,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
			// The field the raft_log message reserves: entries written by
			// older versions carry the compaction results.
			CompactedBlocks: &metastorev1.CompactedBlocks{
				SourceBlocks: &metastorev1.BlockList{Tenant: "tenant-a", Blocks: []string{"b1"}},
				NewBlocks:    []*metastorev1.BlockMeta{{Id: "b2"}},
			},
		}, {
			Name:   "job-running",
			Token:  41,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		}},
		JobCapacity: 3,
	}

	raw, err := proto.Marshal(oldEntry)
	require.NoError(t, err)

	// The FSM command handler decodes the raw entry payload into the request
	// type registered for the command.
	var decoded raft_log.GetCompactionPlanUpdateRequest
	require.NoError(t, proto.Unmarshal(raw, &decoded))

	assert.Equal(t, oldEntry.JobCapacity, decoded.AssignJobsMax)
	require.Len(t, decoded.StatusUpdates, 2)
	assert.Equal(t, "job-done", decoded.StatusUpdates[0].Name)
	assert.Equal(t, uint64(40), decoded.StatusUpdates[0].Token)
	assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS, decoded.StatusUpdates[0].Status)
	assert.Equal(t, "job-running", decoded.StatusUpdates[1].Name)
	assert.Equal(t, uint64(41), decoded.StatusUpdates[1].Token)
	assert.Equal(t, metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS, decoded.StatusUpdates[1].Status)

	// The reserved field is retained as an unknown field, so re-encoding the
	// entry does not discard the data written by the older version.
	assert.NotEmpty(t, decoded.StatusUpdates[0].ProtoReflect().GetUnknown())
	reencoded, err := proto.Marshal(&decoded)
	require.NoError(t, err)
	var roundTripped metastorev1.PollCompactionJobsRequest
	require.NoError(t, proto.Unmarshal(reencoded, &roundTripped))
	assert.True(t, proto.Equal(oldEntry, &roundTripped), "want: %v\ngot:  %v", oldEntry, &roundTripped)
}
