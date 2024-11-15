package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type CompactionCommandLog interface {
	PollCompactionJobs(*metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error)
}

func NewCompactionService(
	logger log.Logger,
	raftLog CompactionCommandLog,
) *CompactionService {
	return &CompactionService{
		logger: logger,
		raft:   raftLog,
	}
}

type CompactionService struct {
	metastorev1.CompactionServiceServer

	logger log.Logger
	raft   CompactionCommandLog
}

func (svc *CompactionService) PollCompactionJobs(
	_ context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	level.Debug(svc.logger).Log(
		"msg", "received poll compaction jobs request",
		"num_updates", len(req.JobStatusUpdates),
		"job_capacity", req.JobCapacity)
	resp, err := svc.raft.PollCompactionJobs(req)
	if err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to apply poll compaction jobs", "err", err)
		return nil, err
	}
	return resp, nil
}
