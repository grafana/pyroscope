package metastore

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/storeutils"
)

type CompactorIndex interface {
	FindBlock(tx *bbolt.Tx, shard uint32, tenant string, block string) *metastorev1.BlockMeta
	FindPartitionMetas(block string) []*index.PartitionMeta
	CreatePartitionKey(string) index.PartitionKey
	ReplaceBlocks(tx *bbolt.Tx, compacted []string, shard uint32, tenant string, blocks []*metastorev1.BlockMeta)
}

type DeletionMarker interface {
	Mark(tx *bbolt.Tx, shard uint32, tenant string, block string, now int64) error
}

type CompactionCommandHandler struct {
	logger  log.Logger
	config  CompactionConfig
	index   CompactorIndex
	marker  DeletionMarker
	metrics *compactionMetrics

	compactionJobBlockQueues map[tenantShard]*compactionJobBlockQueue
	compactionJobQueue       *jobQueue
}

type tenantShard struct {
	tenant string
	shard  uint32
}

type compactionJobBlockQueue struct {
	mu            sync.Mutex
	blocksByLevel map[uint32][]string
}

type pollStateUpdate struct {
	newBlocks          map[tenantShard][]*metastorev1.BlockMeta
	deletedBlocks      map[tenantShard][]string
	updatedBlockQueues map[tenantShard][]uint32
	deletedJobs        map[tenantShard][]string
	newJobs            []string
	updatedJobs        []string
}

func NewCompactionCommandHandler(
	logger log.Logger,
	config CompactionConfig,
	index CompactorIndex,
	marker DeletionMarker,
	reg prometheus.Registerer,
) *CompactionCommandHandler {
	return &CompactionCommandHandler{
		logger:                   logger,
		index:                    index,
		marker:                   marker,
		compactionJobBlockQueues: make(map[tenantShard]*compactionJobBlockQueue),
		compactionJobQueue:       newJobQueue(config.JobLeaseDuration.Nanoseconds()),
		metrics:                  newCompactionMetrics(reg),
		config:                   config,
	}
}

func (h *CompactionCommandHandler) PollCompactionJobs(tx *bbolt.Tx, cmd *raft.Log, request *metastorev1.PollCompactionJobsRequest) (resp *metastorev1.PollCompactionJobsResponse, err error) {
	level.Debug(h.logger).Log(
		"msg", "applying poll compaction jobs",
		"num_updates", len(request.JobStatusUpdates),
		"job_capacity", request.JobCapacity,
		"raft_log_index", cmd.Index)

	stateUpdate := &pollStateUpdate{
		newBlocks:          make(map[tenantShard][]*metastorev1.BlockMeta),
		deletedBlocks:      make(map[tenantShard][]string),
		newJobs:            make([]string, 0),
		updatedBlockQueues: make(map[tenantShard][]uint32),
		deletedJobs:        make(map[tenantShard][]string),
		updatedJobs:        make([]string, 0),
	}

	for _, jobUpdate := range request.JobStatusUpdates {
		job := h.findJob(jobUpdate.JobName)
		if job == nil {
			level.Error(h.logger).Log("msg", "error processing update for compaction job, job not found", "job", jobUpdate.JobName, "err", err)
			continue
		}
		if !h.compactionJobQueue.isOwner(job.Name, jobUpdate.RaftLogIndex) {
			level.Warn(h.logger).Log("msg", "job is not assigned to the worker", "job", jobUpdate.JobName, "raft_log_index", jobUpdate.RaftLogIndex)
			continue
		}
		level.Debug(h.logger).Log("msg", "processing status update for compaction job", "job", jobUpdate.JobName, "status", jobUpdate.Status)
		switch jobUpdate.Status {
		case metastorev1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
			// clean up the job, we don't keep completed jobs around
			h.compactionJobQueue.evict(job.Name, job.RaftLogIndex)
			jobKey := tenantShard{tenant: job.TenantId, shard: job.Shard}
			stateUpdate.deletedJobs[jobKey] = append(stateUpdate.deletedJobs[jobKey], job.Name)
			h.metrics.completedJobs.WithLabelValues(
				fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()

			// next we'll replace source blocks with compacted ones
			h.index.ReplaceBlocks(tx, job.Blocks, job.Shard, job.TenantId, jobUpdate.CompletedJob.Blocks)
			for _, b := range jobUpdate.CompletedJob.Blocks {
				level.Debug(h.logger).Log(
					"msg", "added compacted block",
					"block", b.Id,
					"shard", b.Shard,
					"tenant", b.TenantId,
					"level", b.CompactionLevel,
					"source_job", job.Name)
				blockTenantShard := tenantShard{tenant: b.TenantId, shard: b.Shard}
				stateUpdate.newBlocks[blockTenantShard] = append(stateUpdate.newBlocks[blockTenantShard], b)

				// adding new blocks to the compaction queue
				if jobForNewBlock := h.tryCreateJob(b, jobUpdate.RaftLogIndex); jobForNewBlock != nil {
					h.addCompactionJob(jobForNewBlock)
					stateUpdate.newJobs = append(stateUpdate.newJobs, jobForNewBlock.Name)
					h.metrics.addedJobs.WithLabelValues(
						fmt.Sprint(jobForNewBlock.Shard), jobForNewBlock.TenantId, fmt.Sprint(jobForNewBlock.CompactionLevel)).Inc()
				} else {
					h.addBlockToCompactionJobQueue(b)
				}
				h.metrics.addedBlocks.WithLabelValues(
					fmt.Sprint(b.Shard), b.TenantId, fmt.Sprint(b.CompactionLevel)).Inc()

				stateUpdate.updatedBlockQueues[blockTenantShard] = append(stateUpdate.updatedBlockQueues[blockTenantShard], b.CompactionLevel)
			}
			for _, b := range job.Blocks {
				level.Debug(h.logger).Log(
					"msg", "deleted source block",
					"block", b,
					"shard", job.Shard,
					"tenant", job.TenantId,
					"level", job.CompactionLevel,
					"job", job.Name,
				)
				h.metrics.deletedBlocks.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
				stateUpdate.deletedBlocks[jobKey] = append(stateUpdate.deletedBlocks[jobKey], b)
			}
		case metastorev1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
			level.Debug(h.logger).Log(
				"msg", "compaction job still in progress",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
			)
			h.compactionJobQueue.update(jobUpdate.JobName, cmd.AppendedAt.UnixNano(), jobUpdate.RaftLogIndex)
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
		case metastorev1.CompactionStatus_COMPACTION_STATUS_FAILURE:
			job.Failures += 1
			level.Warn(h.logger).Log(
				"msg", "compaction job failed",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
				"failures", job.Failures,
			)
			if int(job.Failures) >= h.config.JobMaxFailures {
				level.Warn(h.logger).Log(
					"msg", "compaction job reached max failures",
					"job", job.Name,
					"tenant", job.TenantId,
					"shard", job.Shard,
					"level", job.CompactionLevel,
					"failures", job.Failures,
				)
				h.compactionJobQueue.cancel(job.Name)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				h.metrics.discardedJobs.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			} else {
				level.Warn(h.logger).Log(
					"msg", "adding failed compaction job back to the queue",
					"job", job.Name,
					"tenant", job.TenantId,
					"shard", job.Shard,
					"level", job.CompactionLevel,
					"failures", job.Failures,
				)
				h.compactionJobQueue.evict(job.Name, math.MaxInt64)
				job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
				job.RaftLogIndex = 0
				job.LeaseExpiresAt = 0
				h.compactionJobQueue.enqueue(job)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				h.metrics.retriedJobs.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			}
		}
	}

	resp = &metastorev1.PollCompactionJobsResponse{}
	if request.JobCapacity > 0 {
		newJobs := h.findJobsToAssign(int(request.JobCapacity), cmd.Index, cmd.AppendedAt.UnixNano())
		convertedJobs, invalidJobs := h.convertJobs(tx, newJobs)
		resp.CompactionJobs = convertedJobs
		for _, j := range convertedJobs {
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, j.Name)
			h.metrics.assignedJobs.WithLabelValues(
				fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
		}
		for _, j := range invalidJobs {
			key := tenantShard{
				tenant: j.TenantId,
				shard:  j.Shard,
			}
			h.compactionJobQueue.evict(j.Name, math.MaxInt64)
			stateUpdate.deletedJobs[key] = append(stateUpdate.deletedJobs[key], j.Name)
		}
	}

	err = h.writeToDb(tx, stateUpdate)
	if err != nil {
		return nil, err
	}

	for key, blocks := range stateUpdate.deletedBlocks {
		for _, block := range blocks {
			err = h.marker.Mark(tx, key.shard, key.tenant, block, cmd.AppendedAt.UnixNano()/time.Millisecond.Nanoseconds())
			if err != nil {
				return nil, err
			}
		}
	}

	return resp, nil
}

func (h *CompactionCommandHandler) convertJobs(tx *bbolt.Tx, jobs []*compactionpb.CompactionJob) (convertedJobs []*metastorev1.CompactionJob, invalidJobs []*compactionpb.CompactionJob) {
	convertedJobs = make([]*metastorev1.CompactionJob, 0, len(jobs))
	invalidJobs = make([]*compactionpb.CompactionJob, 0, len(jobs))
	for _, job := range jobs {
		// populate block metadata (workers rely on it)
		blocks := make([]*metastorev1.BlockMeta, 0, len(job.Blocks))
		for _, bId := range job.Blocks {
			b := h.index.FindBlock(tx, job.Shard, job.TenantId, bId)
			if b == nil {
				level.Error(h.logger).Log(
					"msg", "failed to populate compaction job details, block not found",
					"block", bId,
					"shard", job.Shard,
					"job", job.Name)
				continue
			}
			blocks = append(blocks, b)
		}
		if len(blocks) == 0 {
			invalidJobs = append(invalidJobs, job)
			level.Warn(h.logger).Log("msg", "skipping assigned compaction job since it has no valid blocks", "job", job.Name)
			continue
		}

		convertedJobs = append(convertedJobs, &metastorev1.CompactionJob{
			Name:   job.Name,
			Blocks: blocks,
			Status: &metastorev1.CompactionJobStatus{
				JobName:      job.Name,
				Status:       metastorev1.CompactionStatus(job.Status),
				RaftLogIndex: job.RaftLogIndex,
				Shard:        job.Shard,
				TenantId:     job.TenantId,
			},
			CompactionLevel: job.CompactionLevel,
			RaftLogIndex:    job.RaftLogIndex,
			Shard:           job.Shard,
			TenantId:        job.TenantId,
		})
	}
	return convertedJobs, invalidJobs
}

func (h *CompactionCommandHandler) findJobsToAssign(jobCapacity int, raftLogIndex uint64, now int64) []*compactionpb.CompactionJob {
	jobsToAssign := make([]*compactionpb.CompactionJob, 0, jobCapacity)
	jobCount, newJobs, inProgressJobs, completedJobs, failedJobs, cancelledJobs := h.compactionJobQueue.stats()
	level.Debug(h.logger).Log(
		"msg", "looking for jobs to assign",
		"job_capacity", jobCapacity,
		"raft_log_index", raftLogIndex,
		"job_queue_size", jobCount,
		"new_jobs_in_queue_count", len(newJobs),
		"in_progress_jobs_in_queue_count", len(inProgressJobs),
		"completed_jobs_in_queue_count", len(completedJobs),
		"failed_jobs_in_queue_count", len(failedJobs),
		"cancelled_jobs_in_queue_count", len(cancelledJobs),
	)

	var j *compactionpb.CompactionJob
	for len(jobsToAssign) < jobCapacity {
		j = h.compactionJobQueue.dequeue(now, raftLogIndex)
		if j == nil {
			break
		}
		level.Debug(h.logger).Log("msg", "assigning job to raftLogIndex", "job", j, "raft_log_index", raftLogIndex)
		jobsToAssign = append(jobsToAssign, j)
	}

	return jobsToAssign
}

func (h *CompactionCommandHandler) writeToDb(tx *bbolt.Tx, sTable *pollStateUpdate) error {
	for _, blocks := range sTable.newBlocks {
		for _, block := range blocks {
			err := persistBlock(tx, h.index.CreatePartitionKey(block.Id), block)
			if err != nil {
				return err
			}
		}
	}
	for key, blocks := range sTable.deletedBlocks {
		for _, block := range blocks {
			err := h.deleteBlock(tx, key.shard, key.tenant, block)
			if err != nil {
				return err
			}
		}
	}
	for _, jobName := range sTable.newJobs {
		job := h.findJob(jobName)
		if job == nil {
			level.Error(h.logger).Log(
				"msg", "a newly added job could not be found",
				"job", jobName,
			)
			continue
		}
		err := h.persistCompactionJob(job.Shard, job.TenantId, job, tx)
		if err != nil {
			return err
		}
	}
	for key, levels := range sTable.updatedBlockQueues {
		for _, l := range levels {
			queue := h.getOrCreateCompactionBlockQueue(key).blocksByLevel[l]
			if queue == nil {
				level.Error(h.logger).Log(
					"msg", "block queue not found",
					"shard", key.shard,
					"tenant", key.tenant,
					"level", l,
				)
				continue
			}
			err := h.persistCompactionJobBlockQueue(key.shard, key.tenant, l, queue, tx)
			if err != nil {
				return err
			}
		}
	}
	for key, jobNames := range sTable.deletedJobs {
		for _, jobName := range jobNames {
			jBucket, jKey := keyForCompactionJob(key.shard, key.tenant, jobName)
			err := updateCompactionJobBucket(tx, jBucket, func(bucket *bbolt.Bucket) error {
				level.Debug(h.logger).Log(
					"msg", "deleting job from storage",
					"job", jobName,
					"shard", key.shard,
					"tenant", key.tenant,
					"storage_bucket", string(jBucket),
					"storage_key", string(jKey))
				return bucket.Delete(jKey)
			})
			if err != nil {
				return err
			}
		}
	}
	for _, jobName := range sTable.updatedJobs {
		job := h.findJob(jobName)
		if job == nil {
			level.Error(h.logger).Log(
				"msg", "an updated job could not be found",
				"job", jobName,
			)
			continue
		}
		err := h.persistCompactionJob(job.Shard, job.TenantId, job, tx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *CompactionCommandHandler) deleteBlock(tx *bbolt.Tx, shardId uint32, tenant, blockId string) error {
	for _, meta := range h.index.FindPartitionMetas(blockId) {
		err := index.UpdateBlockMetadataBucket(tx, meta.Key, shardId, tenant, func(bucket *bbolt.Bucket) error {
			return bucket.Delete([]byte(blockId))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// CompactBlock is the entry point for adding blocks to the compaction flow.
//
// We add the block to a queue identified by the block shard, tenant and compaction level.
//
// If the job creation condition is met (based on the compaction strategy) after adding the block to the queue,
// we create a job and clear the queue.
//
// The method persists the optional job and the queue modification to both the memory state and the db.
func (h *CompactionCommandHandler) CompactBlock(tx *bbolt.Tx, cmd *raft.Log, block *metastorev1.BlockMeta) error {
	// create and store an optional compaction job
	if job := h.tryCreateJob(block, cmd.Index); job != nil {
		if err := h.persistCompactionJob(block.Shard, block.TenantId, job, tx); err != nil {
			return err
		}
		if err := h.persistCompactionJobBlockQueue(block.Shard, block.TenantId, block.CompactionLevel, []string{}, tx); err != nil {
			return err
		}
		h.addCompactionJob(job)
		h.metrics.addedJobs.WithLabelValues(
			fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
	} else {
		key := tenantShard{
			tenant: block.TenantId,
			shard:  block.Shard,
		}
		queue := h.getOrCreateCompactionBlockQueue(key).blocksByLevel[block.CompactionLevel]
		queue = append(queue, block.Id)
		if err := h.persistCompactionJobBlockQueue(block.Shard, block.TenantId, block.CompactionLevel, queue, tx); err != nil {
			return err
		}
		h.addBlockToCompactionJobQueue(block)
	}
	h.metrics.addedBlocks.WithLabelValues(
		fmt.Sprint(block.Shard), block.TenantId, fmt.Sprint(block.CompactionLevel)).Inc()
	return nil
}

func (h *CompactionCommandHandler) tryCreateJob(block *metastorev1.BlockMeta, raftLogIndex uint64) *compactionpb.CompactionJob {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	blockQueue := h.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	if block.CompactionLevel >= globalCompactionStrategy.maxCompactionLevel {
		level.Info(h.logger).Log("msg", "skipping block at max compaction level", "block", block.Id, "compaction_level", block.CompactionLevel)
		return nil
	}

	queuedBlocks := append(blockQueue.blocksByLevel[block.CompactionLevel], block.Id)

	level.Debug(h.logger).Log(
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
		blockIds := make([]string, 0, len(queuedBlocks))
		for _, b := range queuedBlocks {
			blockIds = append(blockIds, b)
		}
		job = &compactionpb.CompactionJob{
			Name:            fmt.Sprintf("L%d-S%d-%d", block.CompactionLevel, block.Shard, calculateHash(queuedBlocks)),
			Blocks:          blockIds,
			Status:          compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED,
			Shard:           block.Shard,
			TenantId:        block.TenantId,
			CompactionLevel: block.CompactionLevel,
		}
		level.Info(h.logger).Log(
			"msg", "created compaction job",
			"job", job.Name,
			"blocks", strings.Join(queuedBlocks, ","),
			"shard", block.Shard,
			"tenant", block.TenantId,
			"compaction_level", block.CompactionLevel)
	}
	return job
}

func (h *CompactionCommandHandler) addCompactionJob(job *compactionpb.CompactionJob) {
	level.Debug(h.logger).Log(
		"msg", "adding compaction job to priority queue",
		"job", job.Name,
		"tenant", job.TenantId,
		"shard", job.Shard,
		"compaction_level", job.CompactionLevel,
	)
	if ok := h.compactionJobQueue.enqueue(job); !ok {
		level.Warn(h.logger).Log("msg", "a compaction job with this name already exists", "job", job.Name)
		return
	}

	// reset the pre-queue for this level
	key := tenantShard{
		tenant: job.TenantId,
		shard:  job.Shard,
	}
	blockQueue := h.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()
	blockQueue.blocksByLevel[job.CompactionLevel] = blockQueue.blocksByLevel[job.CompactionLevel][:0]
}

func (h *CompactionCommandHandler) addBlockToCompactionJobQueue(block *metastorev1.BlockMeta) {
	key := tenantShard{
		tenant: block.TenantId,
		shard:  block.Shard,
	}
	blockQueue := h.getOrCreateCompactionBlockQueue(key)
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	level.Debug(h.logger).Log(
		"msg", "adding block to compaction job block queue",
		"block", block.Id,
		"level", block.CompactionLevel,
		"shard", block.Shard,
		"tenant", block.TenantId)
	blockQueue.blocksByLevel[block.CompactionLevel] = append(blockQueue.blocksByLevel[block.CompactionLevel], block.Id)
}

func calculateHash(blocks []string) uint64 {
	b := make([]byte, 0, 1024)
	for _, blk := range blocks {
		b = append(b, blk...)
	}
	return xxhash.Sum64(b)
}

func (h *CompactionCommandHandler) persistCompactionJob(shard uint32, tenant string, job *compactionpb.CompactionJob, tx *bbolt.Tx) error {
	jobBucketName, jobKey := keyForCompactionJob(shard, tenant, job.Name)
	if err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
		data, _ := job.MarshalVT()
		level.Debug(h.logger).Log("msg", "persisting compaction job", "job", job.Name, "storage_bucket", jobBucketName, "storage_key", jobKey)
		return bucket.Put(jobKey, data)
	}); err != nil {
		return err
	}
	return nil
}

func (h *CompactionCommandHandler) persistCompactionJobBlockQueue(shard uint32, tenant string, compactionLevel uint32, queue []string, tx *bbolt.Tx) error {
	jobBucketName, _ := keyForCompactionJob(shard, tenant, "")
	blockQueue := &compactionpb.CompactionJobBlockQueue{
		CompactionLevel: compactionLevel,
		Shard:           shard,
		Tenant:          tenant,
		Blocks:          queue,
	}
	key := []byte(fmt.Sprintf("%s-%d", compactionBucketJobBlockQueuePrefix, compactionLevel))
	return updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
		data, _ := blockQueue.MarshalVT()
		return bucket.Put(key, data)
	})
}

func (h *CompactionCommandHandler) restoreCompactionPlan(tx *bbolt.Tx) error {
	cdb := tx.Bucket(compactionJobBucketNameBytes)
	return cdb.ForEachBucket(func(name []byte) error {
		shard, tenant, ok := storeutils.ParseTenantShardBucketName(name)
		if !ok {
			_ = level.Error(h.logger).Log("msg", "malformed bucket name", "name", string(name))
			return nil
		}
		key := tenantShard{
			tenant: tenant,
			shard:  shard,
		}
		blockQueue := h.getOrCreateCompactionBlockQueue(key)

		return h.loadCompactionPlan(cdb.Bucket(name), blockQueue)
	})

}

func (h *CompactionCommandHandler) getOrCreateCompactionBlockQueue(key tenantShard) *compactionJobBlockQueue {
	if blockQueue, ok := h.compactionJobBlockQueues[key]; ok {
		return blockQueue
	}
	plan := &compactionJobBlockQueue{
		blocksByLevel: make(map[uint32][]string),
	}
	h.compactionJobBlockQueues[key] = plan
	return plan
}

func (h *CompactionCommandHandler) findJob(name string) *compactionpb.CompactionJob {
	h.compactionJobQueue.mu.Lock()
	defer h.compactionJobQueue.mu.Unlock()
	if jobEntry, exists := h.compactionJobQueue.jobs[name]; exists {
		return jobEntry.CompactionJob
	}
	return nil
}

func (h *CompactionCommandHandler) loadCompactionPlan(b *bbolt.Bucket, blockQueue *compactionJobBlockQueue) error {
	blockQueue.mu.Lock()
	defer blockQueue.mu.Unlock()

	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if strings.HasPrefix(string(k), compactionBucketJobBlockQueuePrefix) {
			var storedBlockQueue compactionpb.CompactionJobBlockQueue
			if err := storedBlockQueue.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to load compaction job block queue %q: %w", string(k), err)
			}
			blockQueue.blocksByLevel[storedBlockQueue.CompactionLevel] = storedBlockQueue.Blocks
			level.Debug(h.logger).Log(
				"msg", "restored compaction job block queue",
				"shard", storedBlockQueue.Shard,
				"compaction_level", storedBlockQueue.CompactionLevel,
				"block_count", len(storedBlockQueue.Blocks),
				"blocks", strings.Join(storedBlockQueue.Blocks, ","))
		} else {
			var job compactionpb.CompactionJob
			if err := job.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to unmarshal job %q: %w", string(k), err)
			}
			h.compactionJobQueue.enqueue(&job)
			level.Debug(h.logger).Log(
				"msg", "restored job into queue",
				"job", job.Name,
				"shard", job.Shard,
				"tenant", job.TenantId,
				"compaction_level", job.CompactionLevel,
				"job_status", job.Status.String(),
				"raft_log_index", job.RaftLogIndex,
				"lease_expires_at", job.LeaseExpiresAt,
				"block_count", len(job.Blocks),
				"blocks", strings.Join(job.Blocks, ","))
		}
	}
	return nil
}

const (
	compactionJobBucketName             = "compaction_job"
	compactionBucketJobBlockQueuePrefix = "compaction-job-block-queue"
)

var compactionJobBucketNameBytes = []byte(compactionJobBucketName)

func updateCompactionJobBucket(tx *bbolt.Tx, name []byte, fn func(*bbolt.Bucket) error) error {
	cdb, err := getCompactionJobBucket(tx)
	if err != nil {
		return err
	}
	bucket, err := storeutils.GetOrCreateSubBucket(cdb, name)
	if err != nil {
		return err
	}
	return fn(bucket)
}

// Bucket           |Key
// [4:shard]<tenant>|[job_name]
func keyForCompactionJob(shard uint32, tenant string, jobName string) (bucket, key []byte) {
	bucket = make([]byte, 4+len(tenant))
	binary.BigEndian.PutUint32(bucket, shard)
	copy(bucket[4:], tenant)
	return bucket, []byte(jobName)
}

func getCompactionJobBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	return tx.CreateBucketIfNotExists(compactionJobBucketNameBytes)
}

func (h *CompactionCommandHandler) Init(tx *bbolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(compactionJobBucketNameBytes); err != nil {
		return err
	}
	return nil
}

func (h *CompactionCommandHandler) Restore(tx *bbolt.Tx) error {
	clear(h.compactionJobBlockQueues)
	h.compactionJobQueue = newJobQueue(h.config.JobLeaseDuration.Nanoseconds())
	return h.restoreCompactionPlan(tx)
}
