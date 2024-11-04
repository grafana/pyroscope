package compaction

import (
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type IndexWriter interface {
	Replace(*bbolt.Tx, CompactedBlocks) error
}

type CompactedBlocks struct {
	Tenant    string
	Shard     uint32
	Source    []string
	Deleted   []string
	Compacted []*metastorev1.BlockMeta
}

type PlanUpdater struct {
	compactor *Compactor
	index     IndexWriter
	store     Store
}

func (u *PlanUpdater) ApplyUpdate(tx *bbolt.Tx, plan *raft_log.CompactionPlanUpdate) error {
	return nil
}

func (u *PlanUpdater) handleStatusSuccess(tx *bbolt.Tx, update *raft_log.CompactionJobState) error {
	// TODO: Load the local version of the job from store.
	//	var stored any
	//
	//	compacted := CompactedBlocks{
	//		Tenant:    stored.Tenant,
	//		Shard:     stored.Shard,
	//		Source:    stored.SourceBlocks,
	//		Deleted:   update.DeletedBlocks,
	//		Compacted: update.CompactedBlocks,
	//	}
	//	if err := u.index.Replace(tx, compacted); err != nil {
	//		return err
	//	}
	//
	//	k := compactionKey{
	//		tenant: stored.Tenant,
	//		shard:  stored.Shard,
	//		level:  stored.CompactionLevel,
	//	}
	//
	//	for _, block := range stored.SourceBlocks {
	//		e := u.compactor.queue.lookup(k, block)
	//		if e == zeroBlockEntry {
	//			continue
	//		}
	//		if err := u.store.DeleteEntry(tx, e.raftIndex, e.id); err != nil {
	//			return err
	//		}
	//	}
	//
	//	return u.store.DeleteJob(update.Name)
	return nil
}
