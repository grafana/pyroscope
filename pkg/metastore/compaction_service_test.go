package metastore

import (
	"context"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
)

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
// only replicated with the final plan update).
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

	svc := NewCompactionService(log.NewNopLogger(), raft)
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
