package metastore

import (
	"encoding/binary"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

var (
	// TODO aleks: for illustration purposes, to be moved externally
	globalCompactionStrategy = compactionStrategy{
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
)

type CompactionConfig struct {
	JobLeaseDuration time.Duration `yaml:"job_lease_duration"`
	JobMaxFailures   int           `yaml:"job_max_failures"`
}

func (cfg *CompactionConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.JobLeaseDuration, prefix+"job-lease-duration", 15*time.Second, "")
	f.IntVar(&cfg.JobMaxFailures, prefix+"job-max-failures", 3, "")
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

type compactionPlanner struct {
	logger  log.Logger
	config  *CompactionConfig
	metrics *compactionMetrics
	blocks  map[tenantShard]*blockQueue
	queue   *jobQueue
}

type blockQueue struct {
	levels map[uint32][]string
}

// CompactBlock is the entry point for adding blocks to the compaction flow.
//
// We add the block to a queue identified by the block shard, tenant and compaction level.
//
// If the job creation condition is met (based on the compaction strategy) after adding the block to the queue,
// we create a job and clear the queue.
//
// The method persists the optional job and the queue modification to both the memory state and the db.
func (m *compactionPlanner) CompactBlock(tx *bbolt.Tx, raftLog *raft.Log, block *metastorev1.BlockMeta) error {
	// create and store an optional compaction job
	if job := m.tryCreateJob(block, raftLog.Index); job != nil {
		if err := persistCompactionJob(tx, block.Shard, block.TenantId, job); err != nil {
			return err
		}
		if err := persistBlockQueue(tx, block.Shard, block.TenantId, block.CompactionLevel, nil); err != nil {
			return err
		}
		m.addCompactionJob(job)
		m.metrics.addedJobs.WithLabelValues(
			fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
	} else {
		key := tenantShard{
			tenant: block.TenantId,
			shard:  block.Shard,
		}
		queue := m.getOrCreateBlockQueue(key).levels[block.CompactionLevel]
		queue = append(queue, block.Id)
		if err := persistBlockQueue(tx, block.Shard, block.TenantId, block.CompactionLevel, queue); err != nil {
			return err
		}
		m.addBlockToCompactionJobQueue(block)
	}
	m.metrics.addedBlocks.WithLabelValues(
		fmt.Sprint(block.Shard), block.TenantId, fmt.Sprint(block.CompactionLevel)).Inc()
	return nil
}

func (m *compactionPlanner) tryCreateJob(block *metastorev1.BlockMeta, raftLogIndex uint64) *compactionpb.CompactionJob {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}

	bq := m.getOrCreateBlockQueue(key)
	if block.CompactionLevel >= globalCompactionStrategy.maxCompactionLevel {
		level.Info(m.logger).Log("msg", "skipping block at max compaction level", "block", block.Id, "compaction_level", block.CompactionLevel)
		return nil
	}

	queuedBlocks := append(bq.levels[block.CompactionLevel], block.Id)

	level.Debug(m.logger).Log(
		"msg", "adding block for compaction",
		"block", block.Id,
		"shard", block.Shard,
		"tenant", block.TenantId,
		"compaction_level", block.CompactionLevel,
		"size", block.Size,
		"queue_size", len(queuedBlocks),
		"raft_log_index", raftLogIndex)

	strategy := getStrategyForLevel(block.CompactionLevel)

	var job *compactionpb.CompactionJob
	if strategy.shouldCreateJob(queuedBlocks) {
		blocks := make([]string, 0, len(queuedBlocks))
		for _, b := range queuedBlocks {
			blocks = append(blocks, b)
		}
		job = &compactionpb.CompactionJob{
			Name:            fmt.Sprintf("%d-L%d-S%d", hash(queuedBlocks), block.CompactionLevel, block.Shard),
			Blocks:          blocks,
			Status:          compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
			Shard:           block.Shard,
			TenantId:        block.TenantId,
			CompactionLevel: block.CompactionLevel,
		}
		level.Info(m.logger).Log(
			"msg", "created compaction job",
			"job", job.Name,
			"blocks", strings.Join(queuedBlocks, ","),
			"shard", block.Shard,
			"tenant", block.TenantId,
			"compaction_level", block.CompactionLevel)
	}
	return job
}

func hash(blocks []string) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk...)
	}
	return xxhash.Sum64(b)
}

func getStrategyForLevel(compactionLevel uint32) compactionLevelStrategy {
	strategy, ok := globalCompactionStrategy.levels[compactionLevel]
	if !ok {
		strategy = globalCompactionStrategy.defaultStrategy
	}
	return strategy
}

func (s compactionLevelStrategy) shouldCreateJob(blocks []string) bool {
	return len(blocks) >= s.maxBlocks
}

func (m *compactionPlanner) addCompactionJob(job *compactionpb.CompactionJob) {
	level.Debug(m.logger).Log(
		"msg", "adding compaction job to priority queue",
		"job", job.Name,
		"tenant", job.TenantId,
		"shard", job.Shard,
		"compaction_level", job.CompactionLevel,
	)
	if ok := m.queue.enqueue(job); !ok {
		level.Warn(m.logger).Log("msg", "a compaction job with this name already exists", "job", job.Name)
		return
	}
	// reset the pre-queue for this level
	key := tenantShard{
		tenant: job.TenantId,
		shard:  job.Shard,
	}
	bq := m.getOrCreateBlockQueue(key)
	bq.levels[job.CompactionLevel] = bq.levels[job.CompactionLevel][:0]
}

func (m *compactionPlanner) addBlockToCompactionJobQueue(block *metastorev1.BlockMeta) {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	bq := m.getOrCreateBlockQueue(key)
	level.Debug(m.logger).Log(
		"msg", "adding block to compaction job block queue",
		"block", block.Id,
		"level", block.CompactionLevel,
		"shard", block.Shard,
		"tenant", block.TenantId)
	bq.levels[block.CompactionLevel] = append(bq.levels[block.CompactionLevel], block.Id)
}

func (m *compactionPlanner) getOrCreateBlockQueue(key tenantShard) *blockQueue {
	if bq, ok := m.blocks[key]; ok {
		return bq
	}
	bq := &blockQueue{levels: make(map[uint32][]string)}
	m.blocks[key] = bq
	return bq
}

func (m *compactionPlanner) findJob(name string) *compactionpb.CompactionJob {
	if jobEntry, exists := m.queue.jobs[name]; exists {
		return jobEntry.CompactionJob
	}
	return nil
}

const (
	compactionBucketJobBlockQueuePrefix = "compaction-job-block-queue"
	compactionJobBucketName             = "compaction_job"
)

var compactionJobBucketNameBytes = []byte(compactionJobBucketName)

func persistCompactionJob(tx *bbolt.Tx, shard uint32, tenant string, job *compactionpb.CompactionJob) error {
	bucket, key := tenantShardBucketAndKey(shard, tenant, job.Name)
	data, _ := job.MarshalVT()
	return compactionJobBucket(tx, bucket).Put(key, data)
}

func persistBlockQueue(tx *bbolt.Tx, shard uint32, tenant string, compactionLevel uint32, queue []string) error {
	bq := &compactionpb.CompactionJobBlockQueue{
		CompactionLevel: compactionLevel,
		Shard:           shard,
		Tenant:          tenant,
		Blocks:          queue,
	}
	bucket, _ := tenantShardBucketAndKey(shard, tenant, "")
	key := []byte(fmt.Sprintf("%s-%d", compactionBucketJobBlockQueuePrefix, compactionLevel))
	value, _ := bq.MarshalVT()
	return compactionJobBucket(tx, bucket).Put(key, value)
}

// Bucket           |Key
// [4:shard]<tenant>|[job_name]
func tenantShardBucketAndKey(shard uint32, tenant string, k string) (bucket, key []byte) {
	bucket = make([]byte, 4+len(tenant))
	binary.BigEndian.PutUint32(bucket, shard)
	copy(bucket[4:], tenant)
	return bucket, []byte(k)
}

func compactionJobBucket(tx *bbolt.Tx, name []byte) *bbolt.Bucket {
	return getOrCreateSubBucket(getOrCreateBucket(tx, compactionJobBucketNameBytes), name)
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
