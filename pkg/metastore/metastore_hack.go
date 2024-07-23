package metastore

import (
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/metastore/raftlogpb"
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
			timestamp := uint64(time.Now().Add(-12 * time.Hour).UnixMilli())
			req := &raftlogpb.TruncateCommand{Timestamp: timestamp}
			_, _, err := applyCommand[*raftlogpb.TruncateCommand, *anypb.Any](m.raft, req, m.config.Raft.ApplyTimeout)
			if err != nil {
				_ = level.Error(m.logger).Log("msg", "failed to apply truncate command", "err", err)
			}
		}
	}
}

func (m *metastoreState) applyTruncate(_ *raft.Log, request *raftlogpb.TruncateCommand) (*anypb.Any, error) {
	m.shardsMutex.Lock()
	var g sync.WaitGroup
	g.Add(len(m.shards))
	for shardID, shard := range m.shards {
		go truncateSegmentsBefore(m.db, m.logger, &g, shardID, shard, request.Timestamp)
	}
	m.shardsMutex.Unlock()
	g.Wait()
	return &anypb.Any{}, nil
}

func truncateSegmentsBefore(
	db *boltdb,
	log log.Logger,
	wg *sync.WaitGroup,
	shardID uint32,
	shard *metastoreShard,
	t uint64,
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
		_ = level.Info(log).Log("msg", "stale segments truncated", "segments", c)
	}()

	bucket, err := getBlockMetadataBucket(tx)
	if err != nil {
		_ = level.Error(log).Log("msg", "failed to get metadata bucket", "err", err)
		return
	}
	shardBucket, _ := keyForBlockMeta(shardID, "", "")
	bucket = bucket.Bucket(shardBucket)

	shard.segmentsMutex.Lock()
	defer shard.segmentsMutex.Unlock()

	for k, segment := range shard.segments {
		if ulid.MustParse(segment.Id).Time() < t {
			if err = bucket.Delete([]byte(segment.Id)); err != nil {
				_ = level.Error(log).Log("msg", "failed to delete stale segments", "err", err)
				return
			}
			delete(shard.segments, k)
			c++
		}
	}
}
