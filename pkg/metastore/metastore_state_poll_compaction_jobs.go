package metastore

import (
	"context"
	"fmt"
	"math"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
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
	return resp, err
}

type jobResult struct {
	newBlocks       []*metastorev1.BlockMeta
	deletedBlocks   []*metastorev1.BlockMeta
	newJobs         []*compactionpb.CompactionJob
	newQueuedBlocks []*metastorev1.BlockMeta
	deletedJobs     []*compactionpb.CompactionJob

	newJobAssignments []*compactionpb.CompactionJob
}

func (m *metastoreState) applyPollCompactionJobs(raft *raft.Log, request *compactorv1.PollCompactionJobsRequest) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	resp = &compactorv1.PollCompactionJobsResponse{}
	level.Debug(m.logger).Log(
		"msg", "applying poll compaction jobs",
		"num_updates", len(request.JobStatusUpdates),
		"job_capacity", request.JobCapacity,
		"raft_log_index", raft.Index)

	jResult := &jobResult{
		newBlocks:         make([]*metastorev1.BlockMeta, 0),
		deletedBlocks:     make([]*metastorev1.BlockMeta, 0),
		newJobs:           make([]*compactionpb.CompactionJob, 0),
		newQueuedBlocks:   make([]*metastorev1.BlockMeta, 0),
		deletedJobs:       make([]*compactionpb.CompactionJob, 0),
		newJobAssignments: make([]*compactionpb.CompactionJob, 0),
	}

	err = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		for _, statusUpdate := range request.JobStatusUpdates {
			job := m.findJob(statusUpdate.JobName)
			if job == nil {
				level.Error(m.logger).Log("msg", "error processing update for compaction job, job not found", "job", statusUpdate.JobName, "err", err)
				continue
			}

			level.Debug(m.logger).Log("msg", "processing status update for compaction job", "job", statusUpdate.JobName, "status", statusUpdate.Status)
			name, _ := keyForCompactionJob(statusUpdate.Shard, statusUpdate.TenantId, statusUpdate.JobName)
			err := updateCompactionJobBucket(tx, name, func(bucket *bbolt.Bucket) error {
				switch statusUpdate.Status { // TODO: handle other cases
				case compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
					err := m.processCompletedJob(tx, job, statusUpdate, jResult, raft.Index)
					if err != nil {
						level.Error(m.logger).Log("msg", "failed to update completed job", "job", job.Name, "err", err)
						return errors.Wrap(err, "failed to update completed job")
					}
				case compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
					if m.compactionJobQueue.isOwner(statusUpdate.JobName, statusUpdate.RaftLogIndex) {
						err := m.persistJobDeadline(tx, job, m.compactionJobQueue.getNewDeadline(raft.AppendedAt.UnixNano()))
						if err != nil {
							return errors.Wrap(err, "failed to update compaction job deadline")
						}
						m.compactionJobQueue.update(statusUpdate.JobName, raft.AppendedAt.UnixNano(), statusUpdate.RaftLogIndex)
					} else {
						level.Warn(m.logger).Log("msg", "compaction job status update rejected", "job", job.Name, "raft_log_index", statusUpdate.RaftLogIndex)
						return errors.New("compaction job status update rejected")
					}
				}
				return nil
			})
			if err != nil {
				level.Error(m.logger).Log("msg", "error processing update for compaction job", "job", job.Name, "err", err)
				continue
			}
		}

		if request.JobCapacity > 0 {
			jResult.newJobAssignments, err = m.assignNewJobs(tx, int(request.JobCapacity), raft.Index, raft.AppendedAt.UnixNano())
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// now update the state
	for _, b := range jResult.newBlocks {
		m.getOrCreateShard(b.Shard).putSegment(b)
		m.compactionMetrics.addedBlocks.WithLabelValues(fmt.Sprint(b.Shard), b.TenantId, fmt.Sprint(b.CompactionLevel)).Inc()
	}

	for _, b := range jResult.deletedBlocks {
		m.getOrCreateShard(b.Shard).deleteSegment(b)
		m.compactionMetrics.deletedBlocks.WithLabelValues(fmt.Sprint(b.Shard), b.TenantId, fmt.Sprint(b.CompactionLevel)).Inc()
	}

	for _, j := range jResult.newJobs {
		m.addCompactionJob(j)
		m.compactionMetrics.addedJobs.WithLabelValues(fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
	}

	for _, b := range jResult.newQueuedBlocks {
		m.addBlockToCompactionJobQueue(b)
		// already counted above
	}

	for _, j := range jResult.deletedJobs {
		m.compactionJobQueue.evict(j.Name, j.RaftLogIndex)
		m.compactionMetrics.completedJobs.WithLabelValues(fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
	}

	resp.CompactionJobs, err = m.convertJobs(jResult.newJobAssignments)
	for _, j := range resp.CompactionJobs {
		m.compactionMetrics.assignedJobs.WithLabelValues(fmt.Sprint(j.Shard), j.TenantId, fmt.Sprint(j.CompactionLevel)).Inc()
	}

	return resp, err
}

func (m *metastoreState) convertJobs(jobs []*compactionpb.CompactionJob) ([]*compactorv1.CompactionJob, error) {
	res := make([]*compactorv1.CompactionJob, 0, len(jobs))
	for _, job := range jobs {
		// populate block metadata (workers rely on it)
		blocks := make([]*metastorev1.BlockMeta, 0, len(job.Blocks))
		for _, bId := range job.Blocks {
			b := m.findBlock(job.Shard, bId)
			if b == nil {
				level.Error(m.logger).Log(
					"msg", "failed to populate job details, block not found",
					"block", bId,
					"shard", job.Shard,
					"job", job.Name)
				continue
			}
			blocks = append(blocks, b)
		}
		if len(blocks) == 0 {
			evicted := m.compactionJobQueue.evict(job.Name, math.MaxInt64)
			level.Warn(m.logger).Log("msg", "skipping assigned compaction job since it has no valid blocks", "job", job.Name, "evicted", evicted)
			continue
		}

		res = append(res, &compactorv1.CompactionJob{
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
	return res, nil
}

func (m *metastoreState) processCompletedJob(
	tx *bbolt.Tx,
	job *compactionpb.CompactionJob,
	update *compactorv1.CompactionJobStatus,
	jResult *jobResult,
	raftLogIndex uint64,
) error {
	ownsJob := m.compactionJobQueue.isOwner(job.Name, update.RaftLogIndex)
	if !ownsJob {
		return errors.New(fmt.Sprintf("deadline exceeded for job with id %s", job.Name))
	}
	jBucket, jKey := keyForCompactionJob(job.Shard, job.TenantId, job.Name)
	err := updateCompactionJobBucket(tx, jBucket, func(bucket *bbolt.Bucket) error {
		return bucket.Delete(jKey)
	})
	if err != nil {
		return err
	}
	jResult.deletedJobs = append(jResult.deletedJobs, job)
	for _, b := range update.CompletedJob.Blocks {
		bName, bKey := keyForBlockMeta(b.Shard, b.TenantId, b.Id)
		err = updateBlockMetadataBucket(tx, bName, func(bucket *bbolt.Bucket) error {
			bValue, _ := b.MarshalVT()
			return bucket.Put(bKey, bValue)
		})
		if err != nil {
			_ = level.Error(m.logger).Log(
				"msg", "failed to add block",
				"block", b.Id,
				"err", err,
			)
			return err
		}
		jResult.newBlocks = append(jResult.newBlocks, b)

		// create and store an optional compaction job
		err, jobToAdd, blockForQueue := m.consumeBlock(b, tx, raftLogIndex)
		if err != nil {
			return err
		}
		if jobToAdd != nil {
			jResult.newJobs = append(jResult.newJobs, jobToAdd)
		} else if blockForQueue != nil {
			jResult.newQueuedBlocks = append(jResult.newQueuedBlocks, blockForQueue)
		}
	}

	// delete source blocks
	bName, _ := keyForBlockMeta(job.Shard, job.TenantId, "")
	err = updateBlockMetadataBucket(tx, bName, func(bucket *bbolt.Bucket) error {
		for _, bId := range job.Blocks {
			level.Debug(m.logger).Log("msg", "deleting block from storage", "block", bId, "compaction_job", job.Name)
			b := m.findBlock(job.Shard, bId)
			if b == nil {
				level.Error(m.logger).Log("msg", "failed to delete block from storage, block not found", "block", bId, "shard", job.Shard)
				return errors.Wrapf(err, "failed to find compaction job source block %s for deletion", bId)
			}

			_, bKey := keyForBlockMeta(b.Shard, b.TenantId, b.Id)
			err := bucket.Delete(bKey)
			if err != nil {
				return errors.Wrapf(err, "failed to delete compaction job source block %s", b.Id)
			}
			jResult.deletedBlocks = append(jResult.deletedBlocks, b)
		}
		return nil
	})
	if err != nil {
		return err
	}
	job.RaftLogIndex = update.RaftLogIndex
	return nil
}

func (m *metastoreState) findBlock(shard uint32, blockId string) *metastorev1.BlockMeta {
	segmentShard := m.getOrCreateShard(shard)
	segmentShard.segmentsMutex.Lock()
	defer segmentShard.segmentsMutex.Unlock()

	return segmentShard.segments[blockId]
}

func (m *metastoreState) persistJobStatus(tx *bbolt.Tx, job *compactionpb.CompactionJob, status compactionpb.CompactionStatus) error {
	return m.persistJob(tx, job, func(storedJob *compactionpb.CompactionJob) {
		storedJob.Status = status
	})
}

func (m *metastoreState) persistJobDeadline(tx *bbolt.Tx, job *compactionpb.CompactionJob, leaseExpiresAt int64) error {
	return m.persistJob(tx, job, func(storedJob *compactionpb.CompactionJob) {
		storedJob.LeaseExpiresAt = leaseExpiresAt
	})
}

func (m *metastoreState) persistJob(tx *bbolt.Tx, job *compactionpb.CompactionJob, fn func(compactionJob *compactionpb.CompactionJob)) error {
	jobBucketName, jobKey := keyForCompactionJob(job.Shard, job.TenantId, job.Name)
	err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
		storedJobData := bucket.Get(jobKey)
		if storedJobData == nil {
			return errors.New("compaction job not found in storage")
		}
		var storedJob compactionpb.CompactionJob
		err := storedJob.UnmarshalVT(storedJobData)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal compaction job data")
		}
		fn(&storedJob)
		jobData, _ := storedJob.MarshalVT()
		return bucket.Put(jobKey, jobData)
	})
	return err
}

func (m *metastoreState) assignNewJobs(tx *bbolt.Tx, jobCapacity int, raftLogIndex uint64, now int64) ([]*compactionpb.CompactionJob, error) {
	jobsToAssign := m.findJobsToAssign(jobCapacity, raftLogIndex, now)
	level.Debug(m.logger).Log("msg", "compaction jobs to assign", "jobs", len(jobsToAssign), "raft_log_index", raftLogIndex, "capacity", jobCapacity)

	for _, job := range jobsToAssign {
		// mark job "in progress"
		err := m.persistJobStatus(tx, job, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS)
		if err != nil {
			level.Error(m.logger).Log("msg", "failed to update job status", "job", job.Name, "err", err)
			// return the job back to the queue
			m.compactionJobQueue.enqueue(job)
			return nil, errors.Wrap(err, "failed to update job status")
		}
	}

	return jobsToAssign, nil
}

func (m *metastoreState) findJobsToAssign(jobCapacity int, raftLogIndex uint64, now int64) []*compactionpb.CompactionJob {
	jobsToAssign := make([]*compactionpb.CompactionJob, 0, jobCapacity)
	jobCount, newJobs, inProgressJobs, completedJobs, failedJobs := m.compactionJobQueue.stats()
	level.Debug(m.logger).Log(
		"msg", "looking for jobs to assign",
		"job_capacity", jobCapacity,
		"raft_log_index", raftLogIndex,
		"job_queue_size", jobCount,
		"new_jobs_in_queue", newJobs,
		"in_progress_jobs_in_queue", inProgressJobs,
		"completed_jobs_in_queue", completedJobs,
		"failed_jobs_in_queue", failedJobs,
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
