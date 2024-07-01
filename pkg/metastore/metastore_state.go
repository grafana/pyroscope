package metastore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type metastoreState struct {
	logger log.Logger

	shardsMutex sync.Mutex
	shards      map[uint32]*metastoreShard

	db *boltdb
}

type metastoreShard struct {
	segmentsMutex sync.Mutex
	segments      map[string]*metastorev1.BlockMeta
}

func newMetastoreState(logger log.Logger, db *boltdb) *metastoreState {
	return &metastoreState{
		logger: logger,
		shards: make(map[uint32]*metastoreShard),
		db:     db,
	}
}

func (m *metastoreState) reset() {
	m.shardsMutex.Lock()
	clear(m.shards)
	m.shardsMutex.Unlock()
}

func (m *metastoreState) getOrCreateShard(shardID uint32) *metastoreShard {
	m.shardsMutex.Lock()
	defer m.shardsMutex.Unlock()
	if shard, ok := m.shards[shardID]; ok {
		return shard
	}
	shard := newMetastoreShard()
	m.shards[shardID] = shard
	return shard
}

func (m *metastoreState) restore(db *boltdb) error {
	m.reset()
	return db.boltdb.View(func(tx *bbolt.Tx) error {
		if err := m.restoreMetadata(tx); err != nil {
			return fmt.Errorf("failed to restore metadata entries: %w", err)
		}
		return nil
	})
}

func (m *metastoreState) restoreMetadata(tx *bbolt.Tx) error {
	mdb, err := getBlockMetadataBucket(tx)
	switch {
	case err == nil:
	case errors.Is(err, bbolt.ErrBucketNotFound):
		return nil
	default:
		return err
	}
	// List shards in the block_metadata bucket:
	// block_metadata/[{shard_id}<tenant_id>]/[block_id]
	// TODO(kolesnikovae): Load concurrently.
	return mdb.ForEachBucket(func(name []byte) error {
		shardID, tenantID, ok := parseBucketName(name)
		if !ok {
			_ = level.Error(m.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		shard := m.getOrCreateShard(shardID)
		if tenantID != "" {
			_ = level.Debug(m.logger).Log("compacted blocks are ignored")
			// TODO: Load tenant blocks.
			return nil
		}
		return shard.loadSegments(mdb.Bucket(name))
	})
}

func newMetastoreShard() *metastoreShard {
	return &metastoreShard{
		segments: make(map[string]*metastorev1.BlockMeta),
	}
}

func (s *metastoreShard) putSegment(segment *metastorev1.BlockMeta) {
	s.segmentsMutex.Lock()
	s.segments[segment.Id] = segment
	s.segmentsMutex.Unlock()
}

func (s *metastoreShard) loadSegments(b *bbolt.Bucket) error {
	s.segmentsMutex.Lock()
	defer s.segmentsMutex.Unlock()
	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var md metastorev1.BlockMeta
		if err := md.UnmarshalVT(v); err != nil {
			return fmt.Errorf("failed to block %q: %w", string(k), err)
		}
		s.segments[md.Id] = &md
	}
	return nil
}
