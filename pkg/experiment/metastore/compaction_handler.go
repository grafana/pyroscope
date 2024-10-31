package metastore

import (
	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type CompactionJobScheduler interface {
	UpdateJobStatus(tx *bbolt.Tx, cmd *raft.Log, status *metastorev1.CompactionJobStatus) error
	AssignJobs(tx *bbolt.Tx, cmd *raft.Log, max uint32) ([]*metastorev1.CompactionJob, error)
}

type PollCompactionJobsRequestHandler struct {
	logger    log.Logger
	scheduler CompactionJobScheduler
}

func (m *PollCompactionJobsRequestHandler) Apply(
	tx *bbolt.Tx, cmd *raft.Log,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	var resp metastorev1.PollCompactionJobsResponse
	for _, status := range req.JobStatusUpdates {
		if err := m.scheduler.UpdateJobStatus(tx, cmd, status); err != nil {
			return nil, err
		}
		if status.Status == metastorev1.CompactionStatus_COMPACTION_STATUS_CANCELLED {
			resp.CancelledJobs = append(resp.CancelledJobs, status.JobName)
		}
	}
	var err error
	resp.CompactionJobs, err = m.scheduler.AssignJobs(tx, cmd, req.JobCapacity)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
