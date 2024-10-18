package metastore

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

func (m *Metastore) PollCompactionJobs(_ context.Context, req *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
	level.Debug(m.logger).Log(
		"msg", "received poll compaction jobs request",
		"num_updates", len(req.JobStatusUpdates),
		"job_capacity", req.JobCapacity,
		"raft_commit_index", m.raft.CommitIndex(),
		"raft_last_index", m.raft.LastIndex(),
		"raft_applied_index", m.raft.AppliedIndex())
	_, resp, err := applyCommand[*compactorv1.PollCompactionJobsRequest, *compactorv1.PollCompactionJobsResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to apply poll compaction jobs", "raft_commit_index", m.raft.CommitIndex(), "err", err)
	}
	return resp, err
}

func (m *metastoreState) applyPollCompactionJobs(raft *raft.Log, request *compactorv1.PollCompactionJobsRequest) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	level.Debug(m.logger).Log(
		"msg", "applying poll compaction jobs",
		"num_updates", len(request.JobStatusUpdates),
		"job_capacity", request.JobCapacity,
		"raft_log_index", raft.Index)

	return m.pollCompactionJobs(request, raft.Index, raft.AppendedAt.UnixNano())
}

type pollStateUpdate struct {
	newBlocks          map[tenantShard][]*metastorev1.BlockMeta
	deletedBlocks      map[tenantShard][]string
	newJobs            []string
	updatedBlockQueues map[tenantShard][]uint32
	deletedJobs        map[tenantShard][]string
	updatedJobs        []string
}

func (m *metastoreState) pollCompactionJobs(request *compactorv1.PollCompactionJobsRequest, raftIndex uint64, raftAppendedAtNanos int64) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	stateUpdate := &pollStateUpdate{
		newBlocks:          make(map[tenantShard][]*metastorev1.BlockMeta),
		deletedBlocks:      make(map[tenantShard][]string),
		newJobs:            make([]string, 0),
		updatedBlockQueues: make(map[tenantShard][]uint32),
		deletedJobs:        make(map[tenantShard][]string),
		updatedJobs:        make([]string, 0),
	}

	for _, jobUpdate := range request.JobStatusUpdates {
		job := m.findJob(jobUpdate.JobName)
		if job == nil {
			level.Error(m.logger).Log("msg", "error processing update for compaction job, job not found", "job", jobUpdate.JobName, "err", err)
			continue
		}
		if !m.compactionJobQueue.isOwner(job.Name, jobUpdate.RaftLogIndex) {
			level.Warn(m.logger).Log("msg", "job is not assigned to the worker", "job", jobUpdate.JobName, "raft_log_index", jobUpdate.RaftLogIndex)
			continue
		}
		level.Debug(m.logger).Log("msg", "processing status update for compaction job", "job", jobUpdate.JobName, "status", jobUpdate.Status)
		switch jobUpdate.Status {
		case compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
			// clean up the job, we don't keep completed jobs around
			m.compactionJobQueue.evict(job.Name, job.RaftLogIndex)
			jobKey := tenantShard{tenant: job.TenantId, shard: job.Shard}
			stateUpdate.deletedJobs[jobKey] = append(stateUpdate.deletedJobs[jobKey], job.Name)
			m.compactionMetrics.completedJobs.WithLabelValues(
				fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()

			// next we'll replace source blocks with compacted ones
			m.index.ReplaceBlocks(job.Blocks, job.Shard, job.TenantId, jobUpdate.CompletedJob.Blocks)
			for _, b := range jobUpdate.CompletedJob.Blocks {
				level.Debug(m.logger).Log(
					"msg", "added compacted block",
					"block", b.Id,
					"shard", b.Shard,
					"tenant", b.TenantId,
					"level", b.CompactionLevel,
					"source_job", job.Name)
				blockTenantShard := tenantShard{tenant: b.TenantId, shard: b.Shard}
				stateUpdate.newBlocks[blockTenantShard] = append(stateUpdate.newBlocks[blockTenantShard], b)

				// adding new blocks to the compaction queue
				if jobForNewBlock := m.tryCreateJob(b, jobUpdate.RaftLogIndex); jobForNewBlock != nil {
					m.addCompactionJob(jobForNewBlock)
					stateUpdate.newJobs = append(stateUpdate.newJobs, jobForNewBlock.Name)
					m.compactionMetrics.addedJobs.WithLabelValues(
						fmt.Sprint(jobForNewBlock.Shard), jobForNewBlock.TenantId, fmt.Sprint(jobForNewBlock.CompactionLevel)).Inc()
				} else {
					m.addBlockToCompactionJobQueue(b)
				}
				m.compactionMetrics.addedBlocks.WithLabelValues(
					fmt.Sprint(b.Shard), b.TenantId, fmt.Sprint(b.CompactionLevel)).Inc()

				stateUpdate.updatedBlockQueues[blockTenantShard] = append(stateUpdate.updatedBlockQueues[blockTenantShard], b.CompactionLevel)
			}
			for _, b := range job.Blocks {
				level.Debug(m.logger).Log(
					"msg", "deleted source block",
					"block", b,
					"shard", job.Shard,
					"tenant", job.TenantId,
					"level", job.CompactionLevel,
					"job", job.Name,
				)
				m.compactionMetrics.deletedBlocks.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
				stateUpdate.deletedBlocks[jobKey] = append(stateUpdate.deletedBlocks[jobKey], b)
			}
		case compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
			level.Debug(m.logger).Log(
				"msg", "compaction job still in progress",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
			)
			m.compactionJobQueue.update(jobUpdate.JobName, raftAppendedAtNanos, jobUpdate.RaftLogIndex)
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
		case compactorv1.CompactionStatus_COMPACTION_STATUS_FAILURE:
			job.Failures += 1
			level.Warn(m.logger).Log(
				"msg", "compaction job failed",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
				"failures", job.Failures,
			)
			if int(job.Failures) >= m.compactionConfig.JobMaxFailures {
				level.Warn(m.logger).Log(
					"msg", "compaction job reached max failures",
					"job", job.Name,
					"tenant", job.TenantId,
					"shard", job.Shard,
					"level", job.CompactionLevel,
					"failures", job.Failures,
				)
				m.compactionJobQueue.cancel(job.Name)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				m.compactionMetrics.discardedJobs.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			} else {
				level.Warn(m.logger).Log(
					"msg", "adding failed compaction job back to the queue",
					"job", job.Name,
					"tenant", job.TenantId,
					"shard", job.Shard,
					"level", job.CompactionLevel,
					"failures", job.Failures,
				)
				m.compactionJobQueue.evict(job.Name, math.MaxInt64)
				job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
				job.RaftLogIndex = 0
				job.LeaseExpiresAt = 0
				m.compactionJobQueue.enqueue(job)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				m.compactionMetrics.retriedJobs.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			}
		}
	}

	resp = &compactorv1.PollCompactionJobsResponse{}
	if request.JobCapacity > 0 {
		newJobs := m.findJobsToAssign(int(request.JobCapacity), raftIndex, raftAppendedAtNanos)
		convertedJobs, invalidJobs := m.convertJobs(newJobs)
		resp.CompactionJobs = convertedJobs
		for _, j := range convertedJobs {
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, j.Name)
			m.compactionMetrics.assignedJobs.WithLabelValues(
				fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
		}
		for _, j := range invalidJobs {
			key := tenantShard{
				tenant: j.TenantId,
				shard:  j.Shard,
			}
			m.compactionJobQueue.evict(j.Name, math.MaxInt64)
			stateUpdate.deletedJobs[key] = append(stateUpdate.deletedJobs[key], j.Name)
		}
	}

	err = m.writeToDb(stateUpdate)
	if err != nil {
		panic(fatalCommandError{fmt.Errorf("error persisting metadata state to db, %w", err)})
	}

	for key, blocks := range stateUpdate.deletedBlocks {
		for _, block := range blocks {
			err = m.deletionMarkers.Mark(key.shard, key.tenant, block, raftAppendedAtNanos/time.Millisecond.Nanoseconds())
			if err != nil {
				panic(fatalCommandError{fmt.Errorf("error persisting block removals, %w", err)})
			}
		}
	}

	return resp, nil
}

func (m *metastoreState) convertJobs(jobs []*compactionpb.CompactionJob) (convertedJobs []*compactorv1.CompactionJob, invalidJobs []*compactionpb.CompactionJob) {
	convertedJobs = make([]*compactorv1.CompactionJob, 0, len(jobs))
	invalidJobs = make([]*compactionpb.CompactionJob, 0, len(jobs))
	for _, job := range jobs {
		// populate block metadata (workers rely on it)
		blocks := make([]*metastorev1.BlockMeta, 0, len(job.Blocks))
		for _, bId := range job.Blocks {
			b := m.index.FindBlock(job.Shard, job.TenantId, bId)
			if b == nil {
				level.Error(m.logger).Log(
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
			level.Warn(m.logger).Log("msg", "skipping assigned compaction job since it has no valid blocks", "job", job.Name)
			continue
		}

		convertedJobs = append(convertedJobs, &compactorv1.CompactionJob{
			Name:   job.Name,
			Blocks: blocks,
			Status: &compactorv1.CompactionJobStatus{
				JobName:      job.Name,
				Status:       compactorv1.CompactionStatus(job.Status),
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

func (m *metastoreState) findJobsToAssign(jobCapacity int, raftLogIndex uint64, now int64) []*compactionpb.CompactionJob {
	jobsToAssign := make([]*compactionpb.CompactionJob, 0, jobCapacity)
	jobCount, newJobs, inProgressJobs, completedJobs, failedJobs, cancelledJobs := m.compactionJobQueue.stats()
	level.Debug(m.logger).Log(
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
		j = m.compactionJobQueue.dequeue(now, raftLogIndex)
		if j == nil {
			break
		}
		level.Debug(m.logger).Log("msg", "assigning job to raftLogIndex", "job", j, "raft_log_index", raftLogIndex)
		jobsToAssign = append(jobsToAssign, j)
	}

	return jobsToAssign
}

func (m *metastoreState) writeToDb(sTable *pollStateUpdate) error {
	return m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		for _, blocks := range sTable.newBlocks {
			for _, block := range blocks {
				err := m.persistBlock(tx, block)
				if err != nil {
					return err
				}
			}
		}
		for key, blocks := range sTable.deletedBlocks {
			for _, block := range blocks {
				err := m.deleteBlock(tx, key.shard, key.tenant, block)
				if err != nil {
					return err
				}
			}
		}
		for _, jobName := range sTable.newJobs {
			job := m.findJob(jobName)
			if job == nil {
				level.Error(m.logger).Log(
					"msg", "a newly added job could not be found",
					"job", jobName,
				)
				continue
			}
			err := m.persistCompactionJob(job.Shard, job.TenantId, job, tx)
			if err != nil {
				return err
			}
		}
		for key, levels := range sTable.updatedBlockQueues {
			for _, l := range levels {
				queue := m.getOrCreateCompactionBlockQueue(key).blocksByLevel[l]
				if queue == nil {
					level.Error(m.logger).Log(
						"msg", "block queue not found",
						"shard", key.shard,
						"tenant", key.tenant,
						"level", l,
					)
					continue
				}
				err := m.persistCompactionJobBlockQueue(key.shard, key.tenant, l, queue, tx)
				if err != nil {
					return err
				}
			}
		}
		for key, jobNames := range sTable.deletedJobs {
			for _, jobName := range jobNames {
				jBucket, jKey := keyForCompactionJob(key.shard, key.tenant, jobName)
				err := updateCompactionJobBucket(tx, jBucket, func(bucket *bbolt.Bucket) error {
					level.Debug(m.logger).Log(
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
			job := m.findJob(jobName)
			if job == nil {
				level.Error(m.logger).Log(
					"msg", "an updated job could not be found",
					"job", jobName,
				)
				continue
			}
			err := m.persistCompactionJob(job.Shard, job.TenantId, job, tx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *metastoreState) deleteBlock(tx *bbolt.Tx, shardId uint32, tenant, blockId string) error {
	metas := m.index.FindPartitionMetas(blockId)
	for _, meta := range metas {
		err := updateBlockMetadataBucket(tx, meta.Key, shardId, tenant, func(bucket *bbolt.Bucket) error {
			return bucket.Delete([]byte(blockId))
		})
		if err != nil {
			return err
		}
	}
	return nil
}
