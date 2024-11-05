package compactor

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type SchedulerIndexReader interface {
}

type compactionSchedule struct {
	tx   *bbolt.Tx
	raft *raft.Raft

	index     SchedulerIndexReader
	scheduler *Scheduler
}

func (p *compactionSchedule) UpdateJob(*metastorev1.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	return nil
}

func (p *compactionSchedule) AssignJob() (*metastorev1.CompactionJob, *raft_log.CompactionJobState) {
	return nil, nil
}
