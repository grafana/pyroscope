package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type PollCompactionJobsCommandLog interface {
	ProposePollCompactionJobs(*metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error)
}

func NewCompactionService(
	logger log.Logger,
	raftLog PollCompactionJobsCommandLog,
) *CompactionService {
	return &CompactionService{
		logger:  logger,
		raftLog: raftLog,
	}
}

type CompactionService struct {
	logger  log.Logger
	raftLog PollCompactionJobsCommandLog
}

func (m *CompactionService) PollCompactionJobs(
	_ context.Context,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	// TODO(kolesnikovae): Validate input.
	resp, err := m.raftLog.ProposePollCompactionJobs(req)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to poll compaction jobs", "err", err)
	}
	// TODO(kolesnikovae): We could wrap the response with additional information such
	//  as metrics, stats, etc. and handle it here to avoid mess with double-counting.
	return resp, err
}
