package metastore

import (
	"encoding/binary"
	"flag"
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
	"github.com/grafana/pyroscope/pkg/util"
)

var (
	_ Compactor           = (*CompactionPlanner)(nil)
	_ CompactionScheduler = (*CompactionPlanner)(nil)
)

// TODO: Make it configurable.
const (
	defaultCompactionJobLeaseDuration = 15 * time.Second
	defaultCompactionJobMaxFailures   = 3
)

type CompactionConfig struct {
	JobLeaseDuration time.Duration `yaml:"job_lease_duration"`
	JobMaxFailures   int           `yaml:"job_max_failures"`
}

func (cfg *CompactionConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.JobLeaseDuration, prefix+"job-lease-duration", 15*time.Second, "")
	f.IntVar(&cfg.JobMaxFailures, prefix+"job-max-failures", 3, "")
}

type PlannerIndex interface {
	InserterIndex
	FindBlock(shard uint32, tenant, block string) *metastorev1.BlockMeta
	ReplaceBlocks(tx *bbolt.Tx, shard uint32, tenant string, new []*metastorev1.BlockMeta, old []string) error
}

// TODO: Implement state loader for the compaction planner.

type CompactionPlanner struct {
	logger   log.Logger
	reg      prometheus.Registerer
	metrics  *compactionMetrics
	blocks   map[tenantShard]*blockQueue
	strategy compactionStrategy
	queue    *jobQueue
	index    PlannerIndex
}

type compactionStrategy struct {
	levels             map[uint32]compactionLevelStrategy
	defaultStrategy    compactionLevelStrategy
	maxCompactionLevel uint32
}

type compactionLevelStrategy struct {
	maxBlocks         int
	maxTotalSizeBytes uint64
}

type tenantShard struct {
	tenant string
	shard  uint32
}

type blockQueue struct {
	levels map[uint32][]string
}

func NewCompactionPlanner(logger log.Logger, reg prometheus.Registerer) *CompactionPlanner {
	s := compactionStrategy{
		levels: map[uint32]compactionLevelStrategy{
			0: {maxBlocks: 20},
		},
		defaultStrategy: compactionLevelStrategy{
			maxBlocks: 10,
		},
		maxCompactionLevel: 3,
		// 0: 0.5
		// 1: 10s
		// 2: 100s
		// 3: 1000s // 16m40s
	}
	p := CompactionPlanner{
		logger:   logger,
		metrics:  newCompactionMetrics(reg),
		blocks:   make(map[tenantShard]*blockQueue),
		queue:    newJobQueue(defaultCompactionJobLeaseDuration.Nanoseconds()),
		strategy: s,
	}
	util.Register(reg, newJobQueueStatsCollector(p.queue))
	return &p
}

func (c *CompactionPlanner) CompactBlock(tx *bbolt.Tx, cmd *raft.Log, block *metastorev1.BlockMeta) error {
	if block.CompactionLevel >= c.strategy.maxCompactionLevel {
		return nil
	}

	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}

	// Put the block into the block queue and check if we need
	// to create a compaction job from the queued blocks.
	queue, err := c.enqueueBlock(tx, key, block)
	if err != nil {
		return err
	}
	c.metrics.addedBlocks.WithLabelValues(compactionMetricDimsBlock(block)...).Inc()
	if !c.getStrategyForLevel(block.CompactionLevel).shouldCreateJob(queue) {
		return nil
	}

	blocks := make([]string, len(queue))
	copy(blocks, queue)
	name := fmt.Sprintf("%d-L%d-S%d", hash(blocks), block.CompactionLevel, block.Shard)
	job := &compactionpb.StoredCompactionJob{
		Name:            name,
		Blocks:          blocks,
		Shard:           block.Shard,
		TenantId:        block.TenantId,
		CompactionLevel: block.CompactionLevel,
		AddedAt:         cmd.AppendedAt.UnixNano(),
	}

	c.queue.enqueue(job)
	if err = c.persistJob(tx, name); err != nil {
		return err
	}

	c.metrics.addedJobs.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
	level.Debug(c.logger).Log("msg", "created compaction job", "job", name, "tenant", block.TenantId)
	return c.cleanBlockQueue(tx, key, block.CompactionLevel)
}

func (c *CompactionPlanner) UpdateJobStatus(tx *bbolt.Tx, cmd *raft.Log, s *metastorev1.CompactionJobStatus) error {
	job := c.findJob(s.JobName)
	if job == nil {
		return ErrJobCanceled
	}

	if !c.queue.isOwner(job.Name, s.RaftLogIndex) {
		level.Warn(c.logger).Log("msg", "job is not assigned to the worker; canceling", "job", s.JobName, "raft_log_index", s.RaftLogIndex)
		return ErrJobCanceled
	}

	level.Debug(c.logger).Log("msg", "processing status update for compaction job", "job", s.JobName, "status", s.Status)
	switch s.Status {
	case metastorev1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
		return c.handleStatusSuccess(tx, cmd, s, job)
	case metastorev1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
		return c.handleStatusInProgress(tx, cmd, s)
	case metastorev1.CompactionStatus_COMPACTION_STATUS_FAILURE:
		return c.handleStatusFailure(tx, s, job)
	default:
		level.Warn(c.logger).Log("msg", "unknown compaction job status", "job", s.JobName)
		return ErrJobCanceled
	}
}

func (c *CompactionPlanner) handleStatusSuccess(
	tx *bbolt.Tx,
	cmd *raft.Log,
	status *metastorev1.CompactionJobStatus,
	job *compactionpb.StoredCompactionJob,
) error {
	if err := c.index.ReplaceBlocks(tx, job.Shard, job.TenantId, status.CompactedBlocks, job.Blocks); err != nil {
		return err
	}
	c.queue.evict(job.Name, job.RaftLogIndex)
	if err := c.deleteJob(tx, job.Name); err != nil {
		return err
	}
	for _, b := range status.CompactedBlocks {
		if err := c.CompactBlock(tx, cmd, b); err != nil {
			return err
		}
	}
	c.metrics.completedJobs.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
	return nil
}

func (c *CompactionPlanner) handleStatusInProgress(
	tx *bbolt.Tx,
	cmd *raft.Log,
	status *metastorev1.CompactionJobStatus,
) error {
	c.queue.update(status.JobName, cmd.AppendedAt.UnixNano(), status.RaftLogIndex)
	return c.persistJob(tx, status.JobName)
}

func (c *CompactionPlanner) handleStatusFailure(
	tx *bbolt.Tx,
	status *metastorev1.CompactionJobStatus,
	job *compactionpb.StoredCompactionJob,
) error {
	job.Failures += 1
	level.Warn(c.logger).Log("msg", "compaction job failed", "job", job.Name, "failures", job.Failures)
	if int(job.Failures) >= defaultCompactionJobMaxFailures {
		level.Error(c.logger).Log("msg", "compaction job reached max failures", "job", job.Name)
		c.queue.cancel(job.Name)
		c.metrics.discardedJobs.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
	} else {
		c.queue.release(job.Name)
		c.metrics.retriedJobs.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
	}
	return c.persistJob(tx, status.JobName)
}

func (c *CompactionPlanner) AssignJob(tx *bbolt.Tx, cmd *raft.Log) (*metastorev1.CompactionJob, error) {
	stored := c.queue.dequeue(cmd.AppendedAt.UnixNano(), cmd.Index)
	if stored == nil {
		// No more jobs to assign.
		return nil, nil
	}
	if err := c.persistJob(tx, stored.Name); err != nil {
		return nil, err
	}
	job, err := c.convertJob(tx, stored)
	if err != nil {
		return nil, err
	}
	level.Debug(c.logger).Log("msg", "job assigned", "job", job.Name, "raft_log_index", cmd.Index)
	c.metrics.assignedJobs.WithLabelValues(compactionMetricDimsJob(stored)...).Inc()
	return job, nil
}

func (c *CompactionPlanner) convertJob(tx *bbolt.Tx, job *compactionpb.StoredCompactionJob) (*metastorev1.CompactionJob, error) {
	blocks := make([]*metastorev1.BlockMeta, len(job.Blocks))
	for _, id := range job.Blocks {
		b := c.index.FindBlock(job.Shard, job.TenantId, id)
		if b == nil {
			level.Error(c.logger).Log("msg", "block not found; deleting the job", "job", job.Name, "block", id)
			c.metrics.invalidJobs.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
			return nil, c.deleteJob(tx, job.Name)
		}
		blocks = append(blocks, b)
	}
	return &metastorev1.CompactionJob{
		Name:    job.Name,
		Options: nil,
		Blocks:  blocks,
		Status: &metastorev1.CompactionJobStatus{
			JobName:      job.Name,
			Status:       metastorev1.CompactionStatus(job.Status),
			RaftLogIndex: job.RaftLogIndex,
			Shard:        job.Shard,
			TenantId:     job.TenantId,
		},
		RaftLogIndex:    job.RaftLogIndex,
		Shard:           job.Shard,
		TenantId:        job.TenantId,
		CompactionLevel: job.CompactionLevel,
	}, nil
}

func hash(blocks []string) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk...)
	}
	return xxhash.Sum64(b)
}

func (c *CompactionPlanner) getStrategyForLevel(compactionLevel uint32) compactionLevelStrategy {
	strategy, ok := c.strategy.levels[compactionLevel]
	if !ok {
		strategy = c.strategy.defaultStrategy
	}
	return strategy
}

func (s compactionLevelStrategy) shouldCreateJob(blocks []string) bool {
	return len(blocks) >= s.maxBlocks
}

func (c *CompactionPlanner) getOrCreateBlockQueue(key tenantShard) *blockQueue {
	if bq, ok := c.blocks[key]; ok {
		return bq
	}
	bq := &blockQueue{levels: make(map[uint32][]string)}
	c.blocks[key] = bq
	return bq
}

func (c *CompactionPlanner) enqueueBlock(tx *bbolt.Tx, ts tenantShard, block *metastorev1.BlockMeta) ([]string, error) {
	bq := c.getOrCreateBlockQueue(ts)
	lvl := block.CompactionLevel
	queue := bq.levels[lvl]
	idx := uint32(len(queue))
	if err := persistCompactionBlockQueueBlock(tx, ts, lvl, idx, block.Id); err != nil {
		return nil, err
	}
	queue = append(queue, block.Id)
	bq.levels[lvl] = queue
	return queue, nil
}

func (c *CompactionPlanner) cleanBlockQueue(tx *bbolt.Tx, ts tenantShard, level uint32) error {
	bq := c.getOrCreateBlockQueue(ts)
	bq.levels[level] = bq.levels[level][:0]
	return deleteCompactionBlockQueue(tx, ts, level)
}

func (c *CompactionPlanner) findJob(name string) *compactionpb.StoredCompactionJob {
	e, ok := c.queue.jobs[name]
	if !ok {
		level.Error(c.logger).Log("msg", "compaction job not found", "job", name)
		c.metrics.invalidJobs.WithLabelValues(compactionMetricDimsJob(e.StoredCompactionJob)...).Inc()
		return nil
	}
	return e.StoredCompactionJob
}

func (c *CompactionPlanner) persistJob(tx *bbolt.Tx, name string) error {
	job := c.findJob(name)
	if job != nil {
		return persistCompactionJob(tx, job)
	}
	return nil
}

func (c *CompactionPlanner) deleteJob(tx *bbolt.Tx, name string) error {
	job := c.findJob(name)
	if job != nil {
		return nil
	}
	if err := deleteCompactionJob(tx, job); err != nil {
		return err
	}
	c.metrics.deletedBlocks.WithLabelValues(compactionMetricDimsJob(job)...).Inc()
	return nil
}

const (
	compactionJobBucketName        = "compaction_job"
	compactionBlockQueueBucketName = "compaction_block_queue"
)

var (
	compactionJobBucketNameBytes        = []byte(compactionJobBucketName)
	compactionBlockQueueBucketNameBytes = []byte(compactionBlockQueueBucketName)
)

func persistCompactionJob(tx *bbolt.Tx, job *compactionpb.StoredCompactionJob) error {
	bucket := tenantShardBucketName(job.Shard, job.TenantId)
	data, _ := job.MarshalVT()
	return compactionJobBucket(tx, bucket).Put([]byte(job.Name), data)
}

func deleteCompactionJob(tx *bbolt.Tx, job *compactionpb.StoredCompactionJob) error {
	bucket := tenantShardBucketName(job.Shard, job.TenantId)
	return compactionJobBucket(tx, bucket).Delete([]byte(job.Name))
}

func compactionJobBucket(tx *bbolt.Tx, name []byte) *bbolt.Bucket {
	parent := getOrCreateBucket(tx, compactionJobBucketNameBytes)
	return getOrCreateSubBucket(parent, name)
}

// Bucket                 | Bucket           |Bucket
// compaction_block_queue | [4:shard]<tenant>|[4:level]
func compactionBlockQueueBucket(tx *bbolt.Tx, ts tenantShard, level uint32) *bbolt.Bucket {
	parent := getOrCreateBucket(tx, compactionBlockQueueBucketNameBytes)
	parent = getOrCreateSubBucket(parent, tenantShardBucketName(ts.shard, ts.tenant))
	name := make([]byte, 4)
	binary.BigEndian.PutUint32(name, level)
	return getOrCreateSubBucket(parent, name)
}

func deleteCompactionBlockQueue(tx *bbolt.Tx, ts tenantShard, level uint32) error {
	parent := getOrCreateBucket(tx, compactionBlockQueueBucketNameBytes)
	parent = getOrCreateSubBucket(parent, tenantShardBucketName(ts.shard, ts.tenant))
	name := make([]byte, 4)
	binary.BigEndian.PutUint32(name, level)
	return parent.Delete(name)
}

func persistCompactionBlockQueueBlock(tx *bbolt.Tx, ts tenantShard, level uint32, index uint32, block string) error {
	key := compactionBlockQueueKey(index, block)
	return compactionBlockQueueBucket(tx, ts, level).Put(key, nil)
}

// Key
// [4:index][block_id]
func compactionBlockQueueKey(index uint32, block string) []byte {
	k := make([]byte, 4+len(block))
	binary.BigEndian.PutUint32(k, index)
	copy(k[4:], block)
	return k
}

// TODO: refactor  -----------------------------------------------------------------------------------------------------

func tenantShardBucketName(shard uint32, tenant string) (bucket []byte) {
	bucket = make([]byte, 4+len(tenant))
	binary.BigEndian.PutUint32(bucket, shard)
	copy(bucket[4:], tenant)
	return bucket
}

func getOrCreateBucket(tx *bbolt.Tx, name []byte) *bbolt.Bucket {
	bucket := tx.Bucket(name)
	if bucket == nil {
		bucket, _ = bucket.CreateBucket(name)
	}
	return bucket
}

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) *bbolt.Bucket {
	bucket := parent.Bucket(name)
	if bucket == nil {
		bucket, _ = parent.CreateBucket(name)
	}
	return bucket
}
