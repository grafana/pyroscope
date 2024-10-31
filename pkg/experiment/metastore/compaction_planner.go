package metastore

import (
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type CompactionPlanner struct {
}

func NewCompactionPlanner() *CompactionPlanner {
	return &CompactionPlanner{}
}

func (c *CompactionPlanner) UpdateJobStatus(tx *bbolt.Tx, now int64, status *metastorev1.CompactionJobStatus) error {
	return nil
}

func (c *CompactionPlanner) AssignJobs(tx *bbolt.Tx, token uint64, now int64, max uint32) ([]*metastorev1.CompactionJob, error) {
	return nil, nil
}

func (c *CompactionPlanner) CompactBlock(*bbolt.Tx, *raft.Log, *metastorev1.BlockMeta) error {
	return nil
}
