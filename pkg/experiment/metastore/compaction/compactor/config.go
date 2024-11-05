package compactor

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// TODO(kolesnikovae): Rename. This is not a strategy but rather a configuration.

type strategy interface {
	// canCompact is called before the block is
	// enqueued to the compaction planing queue.
	canCompact(md *metastorev1.BlockMeta) bool
	// compact is called before and after the
	// block has been added to the batch.
	flush(batch *batch) bool
	// complete is called after the block is added to the job plan.
	complete(*plannedJob) bool
}

const defaultBlockBatchSize = 10

var defaultCompactionStrategy = jobSizeCompactionStrategy{
	maxBlocksPerLevel:  []uint32{20, 10, 10},
	maxBlocksDefault:   defaultBlockBatchSize,
	maxCompactionLevel: 3,
}

type jobSizeCompactionStrategy struct {
	maxBlocksPerLevel  []uint32
	maxBlocksDefault   uint32
	maxCompactionLevel uint32
}

func (s jobSizeCompactionStrategy) maxBlocks(l uint32) uint32 {
	if l >= uint32(len(s.maxBlocksPerLevel)) || len(s.maxBlocksPerLevel) == 0 {
		return s.maxBlocksDefault
	}
	return s.maxBlocksPerLevel[l]
}

func (s jobSizeCompactionStrategy) canCompact(md *metastorev1.BlockMeta) bool {
	return md.CompactionLevel <= s.maxCompactionLevel
}

func (s jobSizeCompactionStrategy) flush(b *batch) bool {
	return b.size >= s.maxBlocks(b.staged.key.level)
}

func (s jobSizeCompactionStrategy) complete(j *plannedJob) bool {
	return uint32(len(j.blocks)) >= s.maxBlocks(j.level)
}
