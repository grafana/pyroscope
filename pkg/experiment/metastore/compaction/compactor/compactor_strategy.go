package compactor

import (
	"time"
)

const (
	defaultBlockBatchSize   = 20
	defaultMaxBlockBatchAge = int64(15 * time.Minute)
)

// TODO: Almost everything here should be level specific.

type Strategy struct {
	MaxBlocksPerLevel []uint64
	MaxBatchAge       int64

	CleanupBatchSize int32
	CleanupDelay     time.Duration

	MaxBlocksDefault   uint64
	CleanupJobMinLevel int32
	CleanupJobMaxLevel int32
}

// compact is called after the block has been added to the batch.
// If the function returns true, the batch is flushed to the global
// queue and becomes available for compaction.
func (s Strategy) flush(b *batch) bool {
	return b.size >= s.maxBlocks(b.staged.key.level)
}

func (s Strategy) flushByAge(b *batch, now int64) bool {
	if s.MaxBatchAge > 0 && b.staged.updatedAt > 0 {
		age := now - b.staged.updatedAt
		return age > s.MaxBatchAge
	}
	return false
}

// complete is called after the block is added to the job plan.
// If the function returns true, the job plan is considered complete
// and the job should be scheduled for execution.
func (s Strategy) complete(j *jobPlan) bool {
	return uint64(len(j.blocks)) >= s.maxBlocks(j.level)
}

func (s Strategy) maxBlocks(l uint32) uint64 {
	if l >= uint32(len(s.MaxBlocksPerLevel)) || len(s.MaxBlocksPerLevel) == 0 {
		return s.MaxBlocksDefault
	}
	return s.MaxBlocksPerLevel[l]
}
