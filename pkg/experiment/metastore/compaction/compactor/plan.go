package compactor

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/util"
)

// plan should be used to prepare the compaction plan update.
// The implementation must have no side effects or alter the
// Compactor in any way.
type plan struct {
	level uint32
	// Read-only.
	tombstones iter.Iterator[*metastorev1.Tombstones]
	compactor  *Compactor
	batches    *batchIter
	blocks     *blockIter
	now        int64
}

func (p *plan) CreateJob() (*raft_log.CompactionJobPlan, error) {
	planned := p.nextJob()
	if planned == nil {
		return nil, nil
	}
	job := raft_log.CompactionJobPlan{
		Name:            planned.name,
		Shard:           planned.shard,
		Tenant:          planned.tenant,
		CompactionLevel: planned.level,
		SourceBlocks:    planned.blocks,
		Tombstones:      planned.tombstones,
	}
	return &job, nil
}

type jobPlan struct {
	compactionKey
	config     *Config
	name       string
	minT       int64
	maxT       int64
	tombstones []*metastorev1.Tombstones
	blocks     []string
}

// Plan compaction of the queued blocks. The algorithm is simple:
//   - Iterate block queues from low levels to higher ones.
//   - Find the oldest batch in the order of arrival and try to compact it.
//   - A batch may not translate into a job (e.g., if some blocks have been
//     removed). Therefore, we navigate to the next batch with the same
//     compaction key in this case.
func (p *plan) nextJob() *jobPlan {
	job := p.newJob()
	for p.level < uint32(len(p.compactor.queue.levels)) {
		if p.batches == nil {
			level := p.compactor.queue.levels[p.level]
			if level == nil {
				p.level++
				continue
			}
			p.batches = newBatchIter(level)
		}

		b, ok := p.batches.next()
		if !ok {
			// We've done with the current level: no more batches
			// in the in-order queue. Move to the next level.
			p.batches = nil
			p.level++
			continue
		}

		// We've found the oldest batch, it's time to plan a job.
		// Job levels are zero based: L0 job means that it includes blocks
		// with compaction level 0. This can be altered (1-based levels):
		// job.level++
		job.reset(b.staged.key)
		p.blocks.setBatch(b)

		// Once we finish with the current block batch, the iterator moves
		// to the next batch–with-the-same-compaction-key, which is not
		// necessarily the next in-order-batch from the batch iterator.
		for {
			if block, ok := p.blocks.peek(); ok {
				if job.tryAdd(block) {
					p.blocks.advance()
					if !job.isComplete() {
						// Try to add more blocks.
						continue
					}
				}
			} else {
				// No more blocks with this compaction key at the level.
				// We may want to force compaction even if the current job
				// is incomplete: e.g., if the blocks remain in the
				// queue for too long. Note that we do not check the block
				// timestamps: we only care when the batch was created.
				if !p.compactor.config.exceedsMaxAge(b, p.now) {
					// The current job plan is to be cancelled, and
					// we move on to the next in-order batch.
					break
				}
			}
			// Typically, we want to proceed to the next compaction key,
			// but if the batch is not empty (i.e., we could not put all
			// the blocks into the job), we must finish it first.
			if p.blocks.more() {
				p.batches.reset(b)
			}
			if len(job.blocks) == 0 {
				// Should not be possible.
				break
			}
			p.getTombstones(job)
			job.finalize()
			return job
		}
	}

	return nil
}

func (p *plan) getTombstones(job *jobPlan) {
	if int32(p.level) > p.compactor.config.CleanupJobMaxLevel {
		return
	}
	if int32(p.level) < p.compactor.config.CleanupJobMinLevel {
		return
	}
	s := int(p.compactor.config.CleanupBatchSize)
	for i := 0; i < s && p.tombstones.Next(); i++ {
		job.tombstones = append(job.tombstones, p.tombstones.At())
	}
}

func (p *plan) newJob() *jobPlan {
	return &jobPlan{
		config: &p.compactor.config,
		blocks: make([]string, 0, defaultBlockBatchSize),
		minT:   math.MaxInt64,
		maxT:   math.MinInt64,
	}
}

func (job *jobPlan) reset(k compactionKey) {
	job.compactionKey = k
	job.blocks = job.blocks[:0]
	job.minT = math.MaxInt64
	job.maxT = math.MinInt64
}

// We may not want to add a bock to the job if it extends the
// compacted block time range beyond the desired limit.
func (job *jobPlan) tryAdd(block string) bool {
	t := util.ULIDStringUnixNano(block)
	if len(job.blocks) > 0 && !job.isInAllowedTimeRange(t) {
		return false
	}
	job.blocks = append(job.blocks, block)
	job.maxT = max(job.maxT, t)
	job.minT = min(job.minT, t)
	return true
}

func (job *jobPlan) isInAllowedTimeRange(t int64) bool {
	if age := job.config.maxAge(job.config.maxLevel()); age > 0 {
		//          minT        maxT
		// --t------|===========|------t--
		//   |      |---------a--------|
		//   |---------b--------|
		a := t - job.minT
		b := job.maxT - t
		if a > age || b > age {
			return false
		}
	}
	return true
}

func (job *jobPlan) isComplete() bool {
	return uint(len(job.blocks)) >= job.config.maxBlocks(job.level)
}

func (job *jobPlan) finalize() {
	nameJob(job)
	job.minT = 0
	job.maxT = 0
	job.config = nil
}

// Job name is a variable length string that should be globally unique
// and is used as a tiebreaker in the compaction job queue ordering.
func nameJob(plan *jobPlan) {
	// Should be on stack; 16b per block; expected ~20 blocks.
	buf := make([]byte, 0, 512)
	for _, b := range plan.blocks {
		buf = append(buf, b...)
	}
	var name strings.Builder
	name.WriteString(fmt.Sprintf("%x", xxhash.Sum64(buf)))
	name.WriteByte('-')
	name.WriteByte('T')
	name.WriteString(plan.tenant)
	name.WriteByte('-')
	name.WriteByte('S')
	name.WriteString(strconv.FormatUint(uint64(plan.shard), 10))
	name.WriteByte('-')
	name.WriteByte('L')
	name.WriteString(strconv.FormatUint(uint64(plan.level), 10))
	plan.name = name.String()
}
