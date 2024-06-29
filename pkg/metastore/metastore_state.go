package metastore

import (
	"sync"

	"github.com/go-kit/log"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type metastoreState struct {
	logger log.Logger

	shardsMutex sync.Mutex
	shards      map[uint64]*metastoreShard
}

type metastoreShard struct {
	segmentsMutex sync.Mutex
	segments      map[string]*metastorev1.BlockMeta
}

func newMetastoreState(logger log.Logger) *metastoreState {
	return &metastoreState{
		logger: logger,
		shards: make(map[uint64]*metastoreShard),
	}
}

func (m *metastoreState) getOrCreateShard(shardID uint64) *metastoreShard {
	m.shardsMutex.Lock()
	defer m.shardsMutex.Unlock()
	if shard, ok := m.shards[shardID]; ok {
		return shard
	}
	shard := newMetastoreShard()
	m.shards[shardID] = shard
	return shard
}
