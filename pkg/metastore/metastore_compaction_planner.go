package metastore

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type compactionPlan struct {
	jobs          map[string]*compactorv1.CompactionJob
	plannedBlocks map[string]interface{}
}

type compactionJob struct {
	name      string
	jobType   string
	blocks    []*metastorev1.BlockMeta
	jobStatus string
}

type Planner struct {
	logger         log.Logger
	metastoreState *metastoreState
	raft           *raft.Raft
	raftConfig     RaftConfig
}

func NewPlanner(state *metastoreState, raft *raft.Raft, raftConfig RaftConfig, logger log.Logger) *Planner {
	state.compactionPlan.plannedBlocks = make(map[string]interface{})
	for _, job := range state.compactionPlan.jobs {
		for _, block := range job.Blocks {
			state.compactionPlan.plannedBlocks[block.Id] = struct{}{}
		}
	}
	return &Planner{
		metastoreState: state,
		raft:           raft,
		raftConfig:     raftConfig,
		logger:         logger,
	}
}

func (c *Planner) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.raft.State() != raft.Leader {
				level.Info(c.logger).Log("msg", "not the leader, skip planning")
				continue
			}
			level.Info(c.logger).Log("msg", "run compaction planning")
			err := c.plan()
			if err != nil {
				level.Warn(c.logger).Log("msg", "failed to run planner", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Planner) plan() error {
	// plan L0 -> L1 jobs only
	c.metastoreState.shardsMutex.Lock()

	blocksToCompact := make(map[uint32][]*metastorev1.BlockMeta, len(c.metastoreState.shards))
	level.Debug(c.logger).Log("msg", "listing shards for compaction", "shard_count", len(c.metastoreState.shards))
	for shardId, shard := range c.metastoreState.shards {
		shard.segmentsMutex.Lock()
		level.Debug(c.logger).Log("msg", "listing segments for compaction", "shard", shardId, "segment_count", len(shard.segments))
		for _, segment := range shard.segments {
			if segment.CompactionLevel > 0 {
				level.Debug(c.logger).Log("msg", "skip segment of higher compaction level", "segment_id", segment.Id)
				continue
			}
			_, ok := c.metastoreState.compactionPlan.plannedBlocks[segment.Id]
			if ok {
				continue
			}
			level.Debug(c.logger).Log("msg", "adding segment to candidates for compaction", "segment_id", segment.Id)
			blocksToCompact[shardId] = append(blocksToCompact[shardId], segment.CloneVT())
		}
		shard.segmentsMutex.Unlock()
	}
	c.metastoreState.shardsMutex.Unlock()

	jobs := make([]*compactorv1.CompactionJob, 0, len(blocksToCompact))
	batchSize := 10
	level.Debug(c.logger).Log("msg", "shards with segments to compact", "shard_count", len(blocksToCompact))
	for shard, blocks := range blocksToCompact {
		level.Debug(c.logger).Log("msg", "segments to compact in shard", "shard", shard, "segment_count", len(blocks))
		// Split in batches if needed
		for i := 0; i < len(blocks); i += batchSize {
			if len(blocks[i:]) >= batchSize {
				id := ulid.MustNew(ulid.Now(), rand.Reader)
				job := &compactorv1.CompactionJob{
					Name:   fmt.Sprintf("L0-%d-%s", shard, id.String()),
					Blocks: blocks[i : i+batchSize],
				}
				level.Info(c.logger).Log("msg", "creating compaction job", "name", job.Name)
				jobs = append(jobs, job)
			}
		}
	}
	level.Info(c.logger).Log("msg", "compaction planning finished", "job_count", len(jobs))

	_, _, err := applyCommand[*compactorv1.AddCompactionJobsRequest, *compactorv1.AddCompactionJobsResponse](
		c.raft, &compactorv1.AddCompactionJobsRequest{Jobs: jobs}, c.raftConfig.ApplyTimeout)

	if err != nil {
		return err
	}
	return nil
}

func (m *Metastore) GetCompactionJobs(_ context.Context, req *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	m.state.compactionPlanMutex.Lock()
	defer m.state.compactionPlanMutex.Unlock()

	resp := &compactorv1.GetCompactionResponse{}
	for _, job := range m.state.compactionPlan.jobs {
		resp.CompactionJobs = append(resp.CompactionJobs, job)
	}

	return resp, nil
}

func (m *Metastore) UpdateJobStatus(_ context.Context, req *compactorv1.UpdateJobStatusRequest) (*compactorv1.UpdateJobStatusResponse, error) {
	_, resp, err := applyCommand[*compactorv1.UpdateJobStatusRequest, *compactorv1.UpdateJobStatusResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	return resp, err
}

func (m *metastoreState) applyAddCompactionJobs(req *compactorv1.AddCompactionJobsRequest) (*compactorv1.AddCompactionJobsResponse, error) {
	m.compactionPlanMutex.Lock()
	defer m.compactionPlanMutex.Unlock()

	level.Debug(m.logger).Log("msg", "applying compaction jobs command", "job_count", len(req.Jobs))

	for _, job := range req.Jobs {
		_, ok := m.compactionPlan.jobs[job.Name]
		if ok {
			level.Warn(m.logger).Log("msg", "cannot add compaction job, a job with that name already exists", "job", job.Name)
			continue
		}
		value, err := job.MarshalVT()
		if err != nil {
			return nil, err
		}
		level.Debug(m.logger).Log("msg", "adding compaction job to storage", "job", job.Name)
		err = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
			return updateCompactionPlanBucket(tx, func(bucket *bbolt.Bucket) error {
				return bucket.Put([]byte(job.Name), value)
			})
		})

		level.Debug(m.logger).Log("msg", "marking job blocks as planned", "job", job.Name, "block_count", len(job.Blocks))
		m.compactionPlan.jobs[job.Name] = job
		for _, block := range job.Blocks {
			m.compactionPlan.plannedBlocks[block.Id] = struct{}{}
		}
	}

	return &compactorv1.AddCompactionJobsResponse{}, nil
}
