package compaction

import (
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type PlannerIndexReader interface {
	LookupBlocks(tx *bbolt.Tx, tenant string, shard uint32, blocks []string) []*metastorev1.BlockMeta
	GetTombstones(tx *bbolt.Tx, tenant string, shard uint32) []string
	LookupJob(tx *bbolt.Tx, tenant string, shard uint32, name string) *metastorev1.CompactionJob
}

type Planner struct {
	index     PlannerIndexReader
	compactor *Compactor
	scheduler *scheduler
}

func NewPlanner(compactor *Compactor, index PlannerIndexReader) *Planner {
	// TODO: Scheduler.
	return &Planner{
		compactor: compactor,
		index:     index,
	}
}

func (p *Planner) NewPlan() Plan {
	return &planUpdate{
		jobBuilder:      newJobBuilder(p.compactor.queue, p.compactor.strategy, p.index),
		scheduleBuilder: newScheduleBuilder(p.scheduler, p.index),
	}
}

type planUpdate struct {
	*jobBuilder
	*scheduleBuilder
}

type jobPlan struct {
	compactionKey
	name   string
	blocks []string
}

type jobBuilder struct {
	index PlannerIndexReader

	strategy strategy
	queue    *queue
	level    uint32

	batches *batchIter
	blocks  *blockIter
}

func newJobBuilder(queue *queue, strategy strategy, index PlannerIndexReader) *jobBuilder {
	return &jobBuilder{
		index:    index,
		blocks:   newBlockIter(),
		strategy: strategy,
		queue:    queue,
	}
}

// Plan compaction of the queued blocks. The algorithm is simple:
//   - Iterate block queues from low levels to higher ones.
//   - Find the oldest batch in the order of arrival and try to compact it.
//   - A batch may not translate into a job (e.g., if some blocks have been
//     removed). Therefore, we navigate to the next batch with the same
//     compaction key in this case.
func (p *jobBuilder) nextJob() *jobPlan {
	var job jobPlan
	for p.level < uint32(len(p.queue.levels)) {
		if p.batches == nil {
			level := p.queue.levels[p.level]
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
		job.compactionKey = b.staged.key
		job.blocks = job.blocks[:0]
		p.blocks.setBatch(b)

		// Once we finish with the current batch blocks, the iterator moves
		// to the next batch–with-the-same-compaction-key, which is not
		// necessarily the next in-order-batch from the batch iterator.
		for {
			block, ok := p.blocks.next()
			if !ok {
				// No more blocks with this compaction key at the level.
				// The current job plan is to be cancelled, and we move
				// on to the next in-order batch.
				break
			}

			job.blocks = append(job.blocks, block)
			if p.strategy.complete(&job) {
				nameJob(&job)
				return &job
			}
		}
	}

	return nil
}

func (p *jobBuilder) CreateJob(tx *bbolt.Tx, _ *raft.Log) *metastorev1.CompactionJob {
	job := p.nextJob()
	if job == nil {
		return nil
	}
	blocks := p.index.LookupBlocks(tx, job.tenant, job.shard, job.blocks)
	tombstones := p.index.GetTombstones(tx, job.tenant, job.shard)
	return &metastorev1.CompactionJob{
		Name:            job.name,
		Shard:           job.shard,
		Tenant:          job.tenant,
		CompactionLevel: job.level,
		SourceBlocks:    blocks,
		Tombstones:      tombstones,
	}
}

func nameJob(plan *jobPlan) {
	// Should be on stack; 16b per block; expected ~20 blocks.
	buf := make([]byte, 0, 512)
	for _, b := range plan.blocks {
		buf = append(buf, b...)
	}
	var name strings.Builder
	name.WriteString(strconv.FormatUint(xxhash.Sum64(buf), 10))
	name.WriteByte('-')
	name.WriteByte('S')
	name.WriteString(strconv.FormatUint(uint64(plan.shard), 10))
	name.WriteByte('-')
	name.WriteByte('L')
	name.WriteString(strconv.FormatUint(uint64(plan.level), 10))
	plan.name = name.String()
}

type scheduleBuilder struct {
	index     PlannerIndexReader
	scheduler *scheduler
}

func newScheduleBuilder(scheduler *scheduler, index PlannerIndexReader) *scheduleBuilder {
	return &scheduleBuilder{
		index:     index,
		scheduler: scheduler,
	}
}

func (p *scheduleBuilder) UpdateJob(tx *bbolt.Tx, log *raft.Log, update *metastorev1.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	return nil
}

func (p *scheduleBuilder) AssignJob(tx *bbolt.Tx, log *raft.Log) (*metastorev1.CompactionJob, *raft_log.CompactionJobState) {
	return nil, nil
}
