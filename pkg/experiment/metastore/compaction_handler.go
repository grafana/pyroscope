package metastore

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var ErrJobCanceled = fmt.Errorf("job cancelled")

type CompactionScheduler interface {
	UpdateJobStatus(tx *bbolt.Tx, cmd *raft.Log, status *metastorev1.CompactionJobStatus) error
	AssignJob(tx *bbolt.Tx, cmd *raft.Log) (*metastorev1.CompactionJob, error)
}

type TombstoneCleaner interface {
	GetTombstones(tx *bbolt.Tx, cmd *raft.Log, size uint32) ([]string, error)
	DeleteTombstones(tx *bbolt.Tx, tombstones []string) error
}

type PollCompactionJobsRequestHandler struct {
	logger    log.Logger
	scheduler CompactionScheduler
	cleaner   TombstoneCleaner
}

func (m *PollCompactionJobsRequestHandler) Apply(
	tx *bbolt.Tx, cmd *raft.Log,
	req *metastorev1.PollCompactionJobsRequest,
) (*metastorev1.PollCompactionJobsResponse, error) {
	var resp metastorev1.PollCompactionJobsResponse
	for _, status := range req.JobStatusUpdates {
		if err := m.scheduler.UpdateJobStatus(tx, cmd, status); err != nil {
			if errors.Is(err, ErrJobCanceled) {
				resp.CancelledJobs = append(resp.CancelledJobs, status.JobName)
				continue
			}
			return nil, err
		}
		if err := m.cleaner.DeleteTombstones(tx, status.DeletedBlocks); err != nil {
			return nil, err
		}
	}
	for len(resp.CompactionJobs) < int(req.JobCapacity) || req.CleanupCapacity > 0 {
		job, err := m.scheduler.AssignJob(tx, cmd)
		if err != nil {
			return nil, err
		}
		job.Tombstones, err = m.cleaner.GetTombstones(tx, cmd, req.CleanupCapacity)
		if err != nil {
			return nil, err
		}
		resp.CompactionJobs = append(resp.CompactionJobs, job)
	}
	return &resp, nil
}
