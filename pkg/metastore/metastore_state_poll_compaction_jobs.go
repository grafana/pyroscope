package metastore

import (
	"context"
	"fmt"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
)

func (m *Metastore) PollCompactionJobs(_ context.Context, req *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
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

func (m *metastoreState) applyPollCompactionJobsStatus(_ *raft.Log, request *compactorv1.PollCompactionJobsRequest) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	resp = &compactorv1.PollCompactionJobsResponse{}
	level.Debug(m.logger).Log(
		"msg", "received poll compaction jobs request",
		"num_updates", len(request.JobStatusUpdates),
		"job_capacity", request.JobCapacity)

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
			// find job
			key := tenantShard{
				tenant: statusUpdate.TenantId,
				shard:  statusUpdate.Shard,
			}
			job := m.findJob(key, statusUpdate.JobName)
			if job == nil {
				return errors.New(fmt.Sprintf("job with name %s not found", statusUpdate.JobName))
			}

			level.Debug(m.logger).Log("msg", "processing status update for compaction job", "job", statusUpdate.JobName, "status", statusUpdate.Status)
			name, _ := keyForCompactionJob(statusUpdate.Shard, statusUpdate.TenantId, statusUpdate.JobName)
			return updateCompactionJobBucket(tx, name, func(bucket *bbolt.Bucket) error {
				switch statusUpdate.Status { // TODO: handle other cases
				case compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS:
					err := m.processCompletedJob(tx, job, statusUpdate, jResult)
					if err != nil {
						level.Error(m.logger).Log("msg", "failed to update completed job", "job", job.Name, "err", err)
						return errors.Wrap(err, "failed to update completed job")
					}
				}
				return nil
			})
		}

		if request.JobCapacity > 0 {
			jResult.newJobAssignments, err = m.assignNewJobs(int(request.JobCapacity), tx)
			if err != nil {
				return err
			}
		}

		return nil
	})

	// now update the state
	if err != nil {
		return nil, err
	}

	for _, b := range jResult.newBlocks {
		m.getOrCreateShard(b.Shard).putSegment(b)
	}

	for _, b := range jResult.deletedBlocks {
		m.getOrCreateShard(b.Shard).deleteSegment(b)
	}

	for _, j := range jResult.newJobs {
		m.addCompactionJob(j)
	}

	for _, b := range jResult.newQueuedBlocks {
		m.addBlockToCompactionJobQueue(b)
	}

	for _, job := range jResult.deletedJobs {
		key := tenantShard{
			tenant: job.TenantId,
			shard:  job.Shard,
		}
		m.getOrCreatePlan(key).deleteJob(job.Name)
	}

	for _, job := range jResult.newJobAssignments {
		key := tenantShard{
			tenant: job.TenantId,
			shard:  job.Shard,
		}
		m.getOrCreatePlan(key).setJobStatus(job.Name, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS)
	}

	resp.CompactionJobs, err = m.convertJobs(jResult.newJobAssignments)

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
				return nil, errors.New(fmt.Sprintf("block with id %s not found", bId))
			}
			blocks = append(blocks, b)
		}

		res = append(res, &compactorv1.CompactionJob{
			Name:   job.Name,
			Blocks: blocks,
			Status: &compactorv1.CompactionJobStatus{
				JobName:     job.Name,
				Status:      compactorv1.CompactionStatus(job.Status),
				CommitIndex: job.CommitIndex,
				Shard:       job.Shard,
				TenantId:    job.TenantId,
			},
			CommitIndex: job.CommitIndex,
			Shard:       job.Shard,
			TenantId:    job.TenantId,
		})
	}
	return res, nil
}

func (m *metastoreState) processCompletedJob(tx *bbolt.Tx, job *compactionpb.CompactionJob, update *compactorv1.CompactionJobStatus, jResult *jobResult) error {
	err := m.persistJobStatus(tx, job, compactionpb.CompactionStatus_COMPACTION_STATUS_SUCCESS)
	if err != nil {
		return err
	}
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
		if job := m.tryCreateJob(b); job != nil {
			level.Debug(m.logger).Log("msg", "persisting compaction job", "job", job.Name)
			jobBucketName, jobKey := keyForCompactionJob(job.Shard, job.TenantId, job.Name)
			err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
				data, _ := job.MarshalVT()
				return bucket.Put(jobKey, data)
			})
			if err != nil {
				return err
			}
			jResult.newJobs = append(jResult.newJobs, job)
		} else {
			jResult.newQueuedBlocks = append(jResult.newQueuedBlocks, b)
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
	jResult.deletedJobs = append(jResult.deletedJobs, job)
	return nil
}

func (m *metastoreState) findBlock(shard uint32, blockId string) *metastorev1.BlockMeta {
	segmentShard := m.getOrCreateShard(shard)
	segmentShard.segmentsMutex.Lock()
	defer segmentShard.segmentsMutex.Unlock()

	return segmentShard.segments[blockId]
}

func (m *metastoreState) persistJobStatus(tx *bbolt.Tx, job *compactionpb.CompactionJob, status compactionpb.CompactionStatus) error {
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
		storedJob.Status = status
		jobData, _ := storedJob.MarshalVT()
		return bucket.Put(jobKey, jobData)
	})
	return err
}

func (m *metastoreState) assignNewJobs(jobCapacity int, tx *bbolt.Tx) ([]*compactionpb.CompactionJob, error) {
	jobsToAssign := m.findJobsToAssign(jobCapacity)

	for _, job := range jobsToAssign {
		// mark job "in progress"
		err := m.persistJobStatus(tx, job, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS)
		if err != nil {
			level.Error(m.logger).Log("msg", "failed to update job status", "job", job.Name, "err", err)
			return nil, errors.Wrap(err, "failed to update job status")
		}
	}

	return jobsToAssign, nil
}

func (m *metastoreState) findJobsToAssign(jobCapacity int) []*compactionpb.CompactionJob {
	m.compactionPlansMutex.Lock()
	defer m.compactionPlansMutex.Unlock()

	jobsToAssign := make([]*compactionpb.CompactionJob, 0, jobCapacity)

	exit := false
	for _, plan := range m.compactionPlans {
		if exit {
			break
		}
		plan.jobsMutex.Lock()
		for _, job := range plan.jobsByName {
			if len(jobsToAssign) >= jobCapacity {
				exit = true
				break
			}
			if job.Status == compactionpb.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED {
				jobsToAssign = append(jobsToAssign, job)
			}
		}
		plan.jobsMutex.Unlock()
	}
	return jobsToAssign
}
