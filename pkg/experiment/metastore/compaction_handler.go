package metastore

import (
	"fmt"
	"math"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

// TODO(kolesnikovae):
//  1. Now we have fatty handler and very thin planner. I believe, it should be
//     the opposite. An interface like the one below may make things cleaner.
//  2. We should not rewrite the whole block queue every time.
//  3. We don't have to create compaction jobs immediately when the block added.
//     It could be done in the AssignCompactionJobs implementation.
//  4. Handler should not access the index directly, but through the planner.
//  5. Deletion marking is a concern of the index implementation.

type CompactionPlanner interface {
	UpdateJobStatus(tx *bbolt.Tx, now int64, status metastorev1.CompactionJobStatus) error
	AssignCompactionJobs(tx *bbolt.Tx, token uint64, now int64, max int) []*metastorev1.CompactionJob
}

type PlannerIndex interface {
	InserterIndex
	FindBlock(shard uint32, tenant, block string) *metastorev1.BlockMeta
	ReplaceBlocks(tx *bbolt.Tx, shard uint32, tenant string, new []*metastorev1.BlockMeta, old []string) error
}

type PollCompactionJobsRequestHandler struct {
	logger log.Logger
	index  PlannerIndex
	*compactionPlanner
}

type pollStateUpdate struct {
	newJobs            []string
	updatedBlockQueues map[tenantShard][]uint32
	deletedJobs        map[tenantShard][]string
	updatedJobs        []string
}

func (m *PollCompactionJobsRequestHandler) Apply(tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	stateUpdate := &pollStateUpdate{
		newJobs:            make([]string, 0),
		updatedBlockQueues: make(map[tenantShard][]uint32),
		deletedJobs:        make(map[tenantShard][]string),
		updatedJobs:        make([]string, 0),
	}
	for _, jobUpdate := range req.JobStatusUpdates {
		// TODO(kolesnikovae): If this is not a terminal status and the job is not
		//  found or not owned, return an error, so the worker stops handling the job.
		job := m.findJob(jobUpdate.JobName)
		if job == nil {
			level.Error(m.logger).Log("msg", "error processing update for compaction job, job not found", "job", jobUpdate.JobName)
			continue
		}
		if !m.queue.isOwner(job.Name, jobUpdate.RaftLogIndex) {
			level.Warn(m.logger).Log("msg", "job is not assigned to the worker", "job", jobUpdate.JobName, "raft_log_index", jobUpdate.RaftLogIndex)
			continue
		}
		level.Debug(m.logger).Log("msg", "processing status update for compaction job", "job", jobUpdate.JobName, "status", jobUpdate.Status)
		switch jobUpdate.Status {
		case metastorev1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
			// clean up the job, we don't keep completed jobs around
			m.queue.evict(job.Name, job.RaftLogIndex)
			jobKey := tenantShard{tenant: job.TenantId, shard: job.Shard}
			stateUpdate.deletedJobs[jobKey] = append(stateUpdate.deletedJobs[jobKey], job.Name)
			m.metrics.completedJobs.WithLabelValues(
				fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			if err := m.index.ReplaceBlocks(tx, job.Shard, job.TenantId, jobUpdate.CompletedJob.Blocks, job.Blocks); err != nil {
				return nil, err
			}
			for _, b := range jobUpdate.CompletedJob.Blocks {
				level.Debug(m.logger).Log(
					"msg", "added compacted block",
					"block", b.Id,
					"shard", b.Shard,
					"tenant", b.TenantId,
					"level", b.CompactionLevel,
					"source_job", job.Name)
				blockTenantShard := tenantShard{tenant: b.TenantId, shard: b.Shard}
				// adding new blocks to the compaction queue
				if jobForNewBlock := m.tryCreateJob(b, jobUpdate.RaftLogIndex); jobForNewBlock != nil {
					m.addCompactionJob(jobForNewBlock)
					stateUpdate.newJobs = append(stateUpdate.newJobs, jobForNewBlock.Name)
					m.metrics.addedJobs.WithLabelValues(
						fmt.Sprint(jobForNewBlock.Shard), jobForNewBlock.TenantId, fmt.Sprint(jobForNewBlock.CompactionLevel)).Inc()
				} else {
					m.addBlockToCompactionJobQueue(b)
				}
				m.metrics.addedBlocks.WithLabelValues(
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
				m.metrics.deletedBlocks.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			}
		case metastorev1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
			level.Debug(m.logger).Log(
				"msg", "compaction job still in progress",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
			)
			m.queue.update(jobUpdate.JobName, cmd.AppendedAt.UnixNano(), jobUpdate.RaftLogIndex)
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
		case metastorev1.CompactionStatus_COMPACTION_STATUS_FAILURE:
			job.Failures += 1
			level.Warn(m.logger).Log(
				"msg", "compaction job failed",
				"job", job.Name,
				"tenant", job.TenantId,
				"shard", job.Shard,
				"level", job.CompactionLevel,
				"failures", job.Failures,
			)
			if int(job.Failures) >= m.config.JobMaxFailures {
				level.Warn(m.logger).Log(
					"msg", "compaction job reached max failures",
					"job", job.Name,
					"tenant", job.TenantId,
					"shard", job.Shard,
					"level", job.CompactionLevel,
					"failures", job.Failures,
				)
				m.queue.cancel(job.Name)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				m.metrics.discardedJobs.WithLabelValues(
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
				// TODO: m.queue.release(job.Name)
				m.queue.evict(job.Name, math.MaxInt64)
				job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
				job.RaftLogIndex = 0
				job.LeaseExpiresAt = 0
				m.queue.enqueue(job)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
				m.metrics.retriedJobs.WithLabelValues(
					fmt.Sprint(job.Shard), job.TenantId, fmt.Sprint(job.CompactionLevel)).Inc()
			}
		}
	}

	resp := &metastorev1.PollCompactionJobsResponse{}
	if req.JobCapacity > 0 {
		newJobs := m.findJobsToAssign(int(req.JobCapacity), cmd.Index, cmd.AppendedAt.UnixNano())
		convertedJobs, invalidJobs := m.convertJobs(newJobs)
		resp.CompactionJobs = convertedJobs
		for _, j := range convertedJobs {
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, j.Name)
			m.metrics.assignedJobs.WithLabelValues(
				fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
		}
		for _, j := range invalidJobs {
			key := tenantShard{
				tenant: j.TenantId,
				shard:  j.Shard,
			}
			m.queue.evict(j.Name, math.MaxInt64)
			stateUpdate.deletedJobs[key] = append(stateUpdate.deletedJobs[key], j.Name)
		}
	}

	err := m.writeToDb(tx, stateUpdate)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (m *PollCompactionJobsRequestHandler) convertJobs(jobs []*compactionpb.CompactionJob) (convertedJobs []*metastorev1.CompactionJob, invalidJobs []*compactionpb.CompactionJob) {
	convertedJobs = make([]*metastorev1.CompactionJob, 0, len(jobs))
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

func (m *PollCompactionJobsRequestHandler) findJobsToAssign(jobCapacity int, raftLogIndex uint64, now int64) []*compactionpb.CompactionJob {
	jobsToAssign := make([]*compactionpb.CompactionJob, 0, jobCapacity)
	jobCount, newJobs, inProgressJobs, completedJobs, failedJobs, cancelledJobs := m.queue.stats()
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
		j = m.queue.dequeue(now, raftLogIndex)
		if j == nil {
			break
		}
		level.Debug(m.logger).Log("msg", "assigning job to raftLogIndex", "job", j, "raft_log_index", raftLogIndex)
		jobsToAssign = append(jobsToAssign, j)
	}

	return jobsToAssign
}

// TODO(kolesnikovae): This can be handled in place.
func (m *PollCompactionJobsRequestHandler) writeToDb(tx *bbolt.Tx, sTable *pollStateUpdate) error {
	for _, jobName := range sTable.newJobs {
		job := m.findJob(jobName)
		if job == nil {
			level.Error(m.logger).Log("msg", "a newly added job could not be found", "job", jobName)
			continue
		}
		if err := persistCompactionJob(tx, job.Shard, job.TenantId, job); err != nil {
			return err
		}
	}
	for key, levels := range sTable.updatedBlockQueues {
		for _, l := range levels {
			queue := m.getOrCreateBlockQueue(key).levels[l]
			if queue == nil {
				level.Error(m.logger).Log(
					"msg", "block queue not found",
					"shard", key.shard,
					"tenant", key.tenant,
					"level", l,
				)
				continue
			}
			if err := persistBlockQueue(tx, key.shard, key.tenant, l, queue); err != nil {
				return err
			}
		}
	}
	for key, jobNames := range sTable.deletedJobs {
		for _, jobName := range jobNames {
			bucket, k := tenantShardBucketAndKey(key.shard, key.tenant, jobName)
			level.Debug(m.logger).Log(
				"msg", "deleting job from storage",
				"job", jobName,
				"shard", key.shard,
				"tenant", key.tenant,
				"storage_bucket", string(bucket),
				"storage_key", string(k),
			)
			if err := compactionJobBucket(tx, bucket).Delete(k); err != nil {
				return err
			}
		}
	}
	for _, jobName := range sTable.updatedJobs {
		job := m.findJob(jobName)
		if job == nil {
			level.Error(m.logger).Log("msg", "an updated job could not be found", "job", jobName)
			continue
		}
		if err := persistCompactionJob(tx, job.Shard, job.TenantId, job); err != nil {
			return err
		}
	}
	return nil
}
