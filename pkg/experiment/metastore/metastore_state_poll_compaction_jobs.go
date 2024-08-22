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
	newBlocks          map[uint32][]string
	deletedBlocks      map[uint32][]string
	newJobs            []string
	updatedBlockQueues map[tenantShard][]uint32
	deletedJobs        map[tenantShard][]string
	updatedJobs        []string
}

func (m *metastoreState) pollCompactionJobs(request *compactorv1.PollCompactionJobsRequest, raftIndex uint64, raftAppendedAtNanos int64) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	stateUpdate := &pollStateUpdate{
		newBlocks:          make(map[uint32][]string),
		deletedBlocks:      make(map[uint32][]string),
		newJobs:            make([]string, 0),
		updatedBlockQueues: make(map[tenantShard][]uint32),
		deletedJobs:        make(map[tenantShard][]string),
		updatedJobs:        make([]string, 0),
	}

	for _, statusUpdate := range request.JobStatusUpdates {
		job := m.findJob(statusUpdate.JobName)
		if job == nil {
			level.Error(m.logger).Log("msg", "error processing update for compaction job, job not found", "job", statusUpdate.JobName, "err", err)
			continue
		}
		if !m.compactionJobQueue.isOwner(job.Name, statusUpdate.RaftLogIndex) {
			level.Warn(m.logger).Log("msg", "job is not assigned to the worker", "job", statusUpdate.JobName, "raft_log_index", statusUpdate.RaftLogIndex)
			continue
		}
		jobKey := tenantShard{
			tenant: job.TenantId,
			shard:  job.Shard,
		}
		level.Debug(m.logger).Log("msg", "processing status update for compaction job", "job", statusUpdate.JobName, "status", statusUpdate.Status)
		switch statusUpdate.Status {
		case compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
			m.compactionJobQueue.evict(job.Name, job.RaftLogIndex)
			stateUpdate.deletedJobs[jobKey] = append(stateUpdate.deletedJobs[jobKey], job.Name)

			for _, b := range statusUpdate.CompletedJob.Blocks {
				m.getOrCreateShard(job.Shard).putSegment(b)
				stateUpdate.newBlocks[job.Shard] = append(stateUpdate.newBlocks[job.Shard], b.Id)

				if job := m.tryCreateJob(b, statusUpdate.RaftLogIndex); job != nil {
					m.addCompactionJob(job)
					stateUpdate.newJobs = append(stateUpdate.newJobs, job.Name)
				} else {
					m.addBlockToCompactionJobQueue(b)
				}
				stateUpdate.updatedBlockQueues[jobKey] = append(stateUpdate.updatedBlockQueues[jobKey], b.CompactionLevel)
			}
			for _, b := range job.Blocks {
				m.getOrCreateShard(job.Shard).deleteSegment(b)
				stateUpdate.deletedBlocks[job.Shard] = append(stateUpdate.deletedBlocks[job.Shard], b)
			}
		case compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS:
			m.compactionJobQueue.update(statusUpdate.JobName, raftAppendedAtNanos, statusUpdate.RaftLogIndex)
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
		case compactorv1.CompactionStatus_COMPACTION_STATUS_FAILURE:
			job.Failures += 1
			if job.Failures > 2 {
				m.compactionJobQueue.evict(job.Name, math.MaxInt64)
				stateUpdate.deletedJobs[jobKey] = append(stateUpdate.deletedJobs[jobKey], job.Name)
			} else {
				m.compactionJobQueue.evict(job.Name, math.MaxInt64)
				job.Status = compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
				job.RaftLogIndex = 0
				job.LeaseExpiresAt = 0
				m.compactionJobQueue.enqueue(job)
				stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, job.Name)
			}
		}
	}

	resp = &compactorv1.PollCompactionJobsResponse{}
	if request.JobCapacity > 0 {
		newJobs := m.findJobsToAssign(int(request.JobCapacity), raftIndex, raftAppendedAtNanos)
		resp.CompactionJobs, err = m.convertJobs(newJobs)
		for _, j := range newJobs {
			stateUpdate.updatedJobs = append(stateUpdate.updatedJobs, j.Name)
		}
		if err != nil {
			return nil, err
		}
	}

	err = m.writeToDb(stateUpdate)
	if err != nil {
		panic(fmt.Errorf("error writing metastore compaction state to db: %w", err))
	}

	return resp, nil
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

func (m *metastoreState) findBlock(shard uint32, blockId string) *metastorev1.BlockMeta {
	segmentShard := m.getOrCreateShard(shard)
	segmentShard.segmentsMutex.Lock()
	defer segmentShard.segmentsMutex.Unlock()

	return segmentShard.segments[blockId]
}

func (m *metastoreState) persistAssignedJob(tx *bbolt.Tx, job *compactionpb.CompactionJob) error {
	return m.persistJobUpdate(tx, job, func(storedJob *compactionpb.CompactionJob) {
		storedJob.Status = job.Status
		storedJob.LeaseExpiresAt = job.LeaseExpiresAt
		storedJob.RaftLogIndex = job.RaftLogIndex
	})
}

func (m *metastoreState) persistJobDeadline(tx *bbolt.Tx, job *compactionpb.CompactionJob, leaseExpiresAt int64) error {
	return m.persistJobUpdate(tx, job, func(storedJob *compactionpb.CompactionJob) {
		storedJob.LeaseExpiresAt = leaseExpiresAt
	})
}

func (m *metastoreState) persistJobUpdate(tx *bbolt.Tx, job *compactionpb.CompactionJob, fn func(compactionJob *compactionpb.CompactionJob)) error {
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
		err := m.persistAssignedJob(tx, job)
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

func (m *metastoreState) writeToDb(sTable *pollStateUpdate) error {
	return m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		for shard, blocks := range sTable.newBlocks {
			for _, b := range blocks {
				block := m.findBlock(shard, b)
				if block == nil {
					return fmt.Errorf("block %s not found in shard %d", b, shard)
				}
				name, key := keyForBlockMeta(shard, "", b)
				err := updateBlockMetadataBucket(tx, name, func(bucket *bbolt.Bucket) error {
					bValue, _ := block.MarshalVT()
					return bucket.Put(key, bValue)
				})
				if err != nil {
					return err
				}
			}
		}
		for shard, blocks := range sTable.deletedBlocks {
			for _, b := range blocks {
				name, key := keyForBlockMeta(shard, "", b)
				err := updateBlockMetadataBucket(tx, name, func(bucket *bbolt.Bucket) error {
					return bucket.Delete(key)
				})
				if err != nil {
					return err
				}
			}
		}
		for _, jobName := range sTable.newJobs {
			job := m.findJob(jobName)
			if job == nil {
				return fmt.Errorf("job %s not found", jobName)
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
					return fmt.Errorf("block queue for %v and level %d not found", key, l)
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
				return fmt.Errorf("job %s not found", jobName)
			}
			err := m.persistCompactionJob(job.Shard, job.TenantId, job, tx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
