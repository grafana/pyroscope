package compactor

// TODO(kolesnikovae): Rename. This is not a strategy but rather a configuration.

type Strategy interface {
	// compact is called after the block has been added to the batch.
	// If the function returns true, the batch is flushed to the global
	// queue and becomes available for compaction.
	flush(*batch) bool

	// complete is called after the block is added to the job plan.
	// If the function returns true, the job plan is considered complete
	// and the job should be scheduled for execution.
	complete(*jobPlan) bool
}

/*

var defaultCompactionStrategy = jobSizeCompactionStrategy{
	maxBlocksPerLevel: []uint32{20, 10, 10},
	maxBlocksDefault:  defaultBlockBatchSize,
}
*/

const defaultBlockBatchSize = 20

type jobSizeCompactionStrategy struct {
	maxBlocksPerLevel []uint32
	maxBlocksDefault  uint32
}

func NewLevelBasedStrategy(maxBlocksPerLevel []uint32, maxBlocksDefault uint32) Strategy {
	return jobSizeCompactionStrategy{
		maxBlocksPerLevel: maxBlocksPerLevel,
		maxBlocksDefault:  maxBlocksDefault,
	}
}

func (s jobSizeCompactionStrategy) maxBlocks(l uint32) uint32 {
	if l >= uint32(len(s.maxBlocksPerLevel)) || len(s.maxBlocksPerLevel) == 0 {
		return s.maxBlocksDefault
	}
	return s.maxBlocksPerLevel[l]
}

func (s jobSizeCompactionStrategy) flush(b *batch) bool {
	return b.size >= s.maxBlocks(b.staged.key.level)
}

func (s jobSizeCompactionStrategy) complete(j *jobPlan) bool {
	return uint32(len(j.blocks)) >= s.maxBlocks(j.level)
}
