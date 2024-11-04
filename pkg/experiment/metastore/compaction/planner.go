package compaction

import (
	"github.com/hashicorp/raft"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type IndexReader interface {
	LookupBlocks(tenant string, shard uint32, blocks []string) []*metastorev1.BlockMeta
	GetTombstones(tenant string, shard uint32) []string
}

type Planner struct {
	compactor *Compactor
	index     IndexReader
}

// Planner should be created by compactor as it owns the queue.
func newPlanner(compactor *Compactor, index IndexReader) *Planner {
	return &Planner{
		compactor: compactor,
		index:     index,
	}
}

func (p *Planner) NewPlan() Plan {
	return &planUpdate{
		builder: newJobBuilder(p.compactor.queue, p.compactor.strategy),
		index:   p.index,
	}
}

type planUpdate struct {
	builder *jobBuilder
	index   IndexReader
}

func (p planUpdate) GetCompactionJob(log *raft.Log) *metastorev1.CompactionJob {
	// TODO: Find a job in queue.

	job := p.builder.nextJob()
	if job == nil {
		return nil
	}
	blocks := p.index.LookupBlocks(job.tenant, job.shard, job.blocks)
	tombstones := p.index.GetTombstones(job.tenant, job.shard)
	return &metastorev1.CompactionJob{
		Name:            job.name,
		Shard:           job.shard,
		Tenant:          job.tenant,
		CompactionLevel: job.level,
		SourceBlocks:    blocks,
		Tombstones:      tombstones,
	}
}

func (p planUpdate) AssignJob(log *raft.Log, job *metastorev1.CompactionJob) *raft_log.CompactionJobState {
	//TODO implement me
	panic("implement me")
}

func (p planUpdate) UpdateJob(log *raft.Log, update *metastorev1.CompactionJobStatusUpdate) *raft_log.CompactionJobState {
	//TODO implement me
	panic("implement me")
}
