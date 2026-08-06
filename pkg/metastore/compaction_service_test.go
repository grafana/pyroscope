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
