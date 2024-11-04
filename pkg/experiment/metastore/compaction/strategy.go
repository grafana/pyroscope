package compaction

import metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"

type compactionStrategy interface {
	// canCompact is called before the block is
	// enqueued to the compaction planing queue.
	canCompact(md *metastorev1.BlockMeta) bool
	batchStrategy(uint32) batchStrategy
}

type batchStrategy interface {
	// compact is called before and after the
	// block has been added to the batch.
	flush(batch *batch) bool
}

const defaultBlockBatchSize = 10

var defaultCompactionStrategy = simpleCompactionStrategy{
	maxBlocksPerLevel:  []uint32{20, 10, 10},
	maxBlocksDefault:   defaultBlockBatchSize,
	maxCompactionLevel: 3,
}

type simpleCompactionStrategy struct {
	maxBlocksPerLevel  []uint32
	maxBlocksDefault   uint32
	maxCompactionLevel uint32
}

func (s simpleCompactionStrategy) canCompact(md *metastorev1.BlockMeta) bool {
	return md.CompactionLevel <= s.maxCompactionLevel
}

func (s simpleCompactionStrategy) batchStrategy(l uint32) batchStrategy {
	if l >= uint32(len(s.maxBlocksPerLevel)) || len(s.maxBlocksPerLevel) == 0 {
		return blockBatchSize{num: s.maxBlocksDefault}
	}
	return blockBatchSize{num: s.maxBlocksPerLevel[l]}
}

// TODO(kolesnikovae): Check time range the batch covers.
//   We should not compact blocks that are too far apart.

type blockBatchSize struct{ num uint32 }

func (s blockBatchSize) flush(b *batch) bool { return b.size >= s.num }
