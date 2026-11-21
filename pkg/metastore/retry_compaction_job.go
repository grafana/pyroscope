package metastore

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
)

func (svc *CompactionService) RetryCompactionJob(ctx context.Context, req *metastorev1.RetryCompactionJobRequest) (*metastorev1.RetryCompactionJobResponse, error) {
	if strings.TrimSpace(req.GetName()) == "" || req.GetToken() == 0 {
		return nil, status.Error(codes.InvalidArgument, "job name and ownership token are required")
	}

	// Serialize the prepare/commit pair with worker polling and dataset deletion.
	svc.mu.Lock()
	defer svc.mu.Unlock()

	result, err := svc.raft.Propose(ctx,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE),
		&raft_log.GetCompactionPlanUpdateRequest{RetryJob: req})
	if err != nil {
		return nil, err
	}
	prepared := result.(*raft_log.GetCompactionPlanUpdateResponse)
	if len(prepared.GetPlanUpdate().GetUpdatedJobs()) != 1 {
		return nil, status.Error(codes.FailedPrecondition, "job changed or is not exhausted with an expired lease, refresh before retrying")
	}
	result, err = svc.raft.Propose(ctx,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN),
		&raft_log.UpdateCompactionPlanRequest{Term: prepared.Term, PlanUpdate: prepared.PlanUpdate})
	if err != nil {
		return nil, err
	}
	if result.(*raft_log.UpdateCompactionPlanResponse).GetPlanUpdate() == nil {
		return nil, status.Error(codes.FailedPrecondition, "compaction plan changed, refresh before retrying")
	}
	return &metastorev1.RetryCompactionJobResponse{}, nil
}
