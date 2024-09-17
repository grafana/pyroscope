package metastore

import (
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
)

// FIXME(kolesnikovae):
//   Remove once compaction is implemented.
//   Or use index instead of the timestamp.

func (m *Metastore) cleanupLoop() {
	t := time.NewTicker(10 * time.Minute)
	defer func() {
		t.Stop()
		m.wg.Done()
	}()
	for {
		select {
		case <-m.done:
			return
		case <-t.C:
			if m.raft.State() != raft.Leader {
				continue
			}
			timestamp := uint64(time.Now().Add(-7 * 24 * time.Hour).UnixMilli())
			req := &raftlogpb.TruncateCommand{Timestamp: timestamp}
			_, _, err := applyCommand[*raftlogpb.TruncateCommand, *anypb.Any](m.raft, req, m.config.Raft.ApplyTimeout)
			if err != nil {
				_ = level.Error(m.logger).Log("msg", "failed to apply truncate command", "err", err)
			}
		}
	}
}

func (m *metastoreState) applyTruncate(_ *raft.Log, request *raftlogpb.TruncateCommand) (*anypb.Any, error) {
	m.index.run(func() {
		toDelete := make([]PartitionKey, 0)
		for key, partition := range m.index.partitionMap {
			if uint64(partition.ts.UnixMilli()) < request.Timestamp {
				toDelete = append(toDelete, key)
			}
		}
		var g sync.WaitGroup
		g.Add(len(toDelete))
		for _, key := range toDelete {
			go truncatePartition(m.db, m.index, m.logger, &g, key)
		}
		g.Wait()
	})

	return &anypb.Any{}, nil
}

func truncatePartition(
	db *boltdb,
	index *index,
	log log.Logger,
	wg *sync.WaitGroup,
	key PartitionKey,
) {
	defer wg.Done()
	var c int
	tx, err := db.boltdb.Begin(true)
	if err != nil {
		_ = level.Error(log).Log("msg", "failed to start transaction", "err", err)
		return
	}
	defer func() {
		if err = tx.Commit(); err != nil {
			_ = level.Error(log).Log("msg", "failed to commit transaction", "err", err)
			return
		}
		_ = level.Info(log).Log("msg", "stale partitions truncated", "segments", c)
	}()

	bucket, err := getPartitionBucket(tx)
	if err != nil {
		_ = level.Error(log).Log("msg", "failed to get metadata bucket", "err", err)
		return
	}
	index.run(func() {
		delete(index.partitionMap, key)
		if err = bucket.Delete([]byte(key)); err != nil {
			_ = level.Error(log).Log(
				"msg", "failed to delete stale partition",
				"err", err,
				"partition", key)
			return
		}
	})
}
