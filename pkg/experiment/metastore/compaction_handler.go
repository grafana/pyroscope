package metastore

import (
	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type CompactionJobScheduler interface {
	UpdateJobStatus(tx *bbolt.Tx, now int64, status *metastorev1.CompactionJobStatus) error
	AssignJobs(tx *bbolt.Tx, token uint64, now int64, max uint32) ([]*metastorev1.CompactionJob, error)
}

type PollCompactionJobsRequestHandler struct {
	logger    log.Logger
	scheduler CompactionJobScheduler
}

func (m *PollCompactionJobsRequestHandler) Apply(
	tx *bbolt.Tx, cmd *raft.Log,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	// Instead of the real-time notion of now, we're using the time
	// when the command was appended to the log to ensure consistent
	// behaviour across all replicas.
	now := cmd.AppendedAt.UnixNano()
	token := cmd.Index

	var resp metastorev1.PollCompactionJobsResponse
	for _, status := range req.JobStatusUpdates {
		if err := m.scheduler.UpdateJobStatus(tx, now, status); err != nil {
			return nil, err
		}
		if status.Status == metastorev1.CompactionStatus_COMPACTION_STATUS_CANCELLED {
			resp.CancelledJobs = append(resp.CancelledJobs, status.JobName)
		}
	}

	var err error
	resp.CompactionJobs, err = m.scheduler.AssignJobs(tx, token, now, req.JobCapacity)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
