package metastore

import (
	"context"
	"fmt"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
)

func (m *Metastore) PollCompactionJobs(_ context.Context, req *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
	_, resp, err := applyCommand[*compactorv1.PollCompactionJobsRequest, *compactorv1.PollCompactionJobsResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	return resp, err
}

func (m *metastoreState) applyPollCompactionJobsStatus(request *compactorv1.PollCompactionJobsRequest) (resp *compactorv1.PollCompactionJobsResponse, err error) {
	resp = &compactorv1.PollCompactionJobsResponse{}
	level.Debug(m.logger).Log("msg", "received poll compaction jobs request", "num_updates", len(request.JobStatusUpdates))

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
					err := m.processCompletedJob(tx, job, statusUpdate)
					if err != nil {
						level.Error(m.logger).Log("msg", "failed to update completed job", "job", job.Name, "err", err)
						return errors.Wrap(err, "failed to update completed job")
					}
				}

				return nil
			})
		}

		if request.JobCapacity > 0 {
			jobs, err := m.assignNewJobs(int(request.JobCapacity), tx)
			if err != nil {
				return err
			}
			resp.CompactionJobs = jobs
		}

		return nil
	})

	return resp, err
}

func (m *metastoreState) processCompletedJob(tx *bbolt.Tx, job *compactorv1.CompactionJob, update *compactorv1.CompactionJobStatus) error {
	err := m.persistJobStatus(tx, job, update)
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
			m.addCompactionJob(job)
		} else {
			m.addBlockToCompactionJobQueue(b)
		}
		m.getOrCreateShard(b.Shard).putSegment(b)
	}

	// delete source blocks
	bName, _ := keyForBlockMeta(job.Shard, job.TenantId, "")
	err = updateBlockMetadataBucket(tx, bName, func(bucket *bbolt.Bucket) error {
		for _, b := range job.Blocks {
			level.Debug(m.logger).Log("msg", "deleting block from storage", "block", b.Id, "compaction_job", job.Name)
			_, bKey := keyForBlockMeta(b.Shard, b.TenantId, b.Id)
			err := bucket.Delete(bKey)
			if err != nil {
				return errors.Wrapf(err, "failed to delete compaction job source block %s", b.Id)
			}
		}
		return nil
	})
	for _, b := range job.Blocks {
		level.Debug(m.logger).Log("msg", "deleting block from state", "block", b.Id, "compaction_job", job.Name)
		m.getOrCreateShard(b.Shard).deleteSegment(b)
	}

	// TODO: Remove job

	return nil
}

func (m *metastoreState) persistJobStatus(tx *bbolt.Tx, job *compactorv1.CompactionJob, update *compactorv1.CompactionJobStatus) error {
	jobBucketName, jobKey := keyForCompactionJob(job.Shard, job.TenantId, job.Name)
	err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
		storedJobData := bucket.Get(jobKey)
		if storedJobData == nil {
			return errors.New("compaction job not found in storage")
		}
		var storedJob compactorv1.CompactionJob
		err := storedJob.UnmarshalVT(storedJobData)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal compaction job data")
		}
		storedJob.Status.Status = update.Status
		storedJob.Status.CompletedJob = update.CompletedJob
		jobData, _ := storedJob.MarshalVT()
		return bucket.Put(jobKey, jobData)
	})
	return err
}

func (m *metastoreState) assignNewJobs(jobCapacity int, tx *bbolt.Tx) ([]*compactorv1.CompactionJob, error) {
	jobsToAssign := make([]*compactorv1.CompactionJob, 0, jobCapacity)
	for job := range m.getJobs(compactorv1.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED, func(job *compactorv1.CompactionJob) bool {
		return len(jobsToAssign) >= jobCapacity
	}) {
		jobsToAssign = append(jobsToAssign, job)
	}

	if len(jobsToAssign) > 0 {
		level.Info(m.logger).Log("msg", "compaction jobs found", "jobs", len(jobsToAssign))

		for _, job := range jobsToAssign {
			err := m.persistJobStatus(tx, job, &compactorv1.CompactionJobStatus{
				Status: compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS,
			})
			if err != nil {
				level.Error(m.logger).Log("msg", "failed to assign job", "job", job.Name, "err", err)
				return nil, errors.Wrap(err, "failed to update completed job")
			}
		}
	}

	return jobsToAssign, nil
}
