package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
)

func NewCompactionService(
	logger log.Logger,
	raftLog Raft,
) *CompactionService {
	return &CompactionService{
		logger: logger,
		raft:   raftLog,
	}
}

type CompactionService struct {
	metastorev1.CompactionServiceServer

	logger log.Logger
	raft   Raft
}

func (svc *CompactionService) PollCompactionJobs(
	_ context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	level.Debug(svc.logger).Log(
		"msg", "received poll compaction jobs request",
		"num_updates", len(req.JobStatusUpdates),
		"job_capacity", req.JobCapacity)
	resp, err := svc.raft.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_POLL_COMPACTION_JOBS), req)
	if err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to apply poll compaction jobs", "err", err)
		return nil, err
	}
	return resp.(*metastorev1.PollCompactionJobsResponse), nil
}
