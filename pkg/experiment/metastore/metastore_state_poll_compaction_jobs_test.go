package metastore

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compactionpb"
)

func Test_JobAssignments(t *testing.T) {
	// add enough blocks to create 2 jobs
	m := initState(t)
	addLevel0Blocks(m, 40)
	require.Equal(t, 2, len(m.compactionJobQueue.jobs))

	// a worker asks for and gets 2 jobs assigned
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 2}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.CompactionJobs))
	for _, job := range resp.CompactionJobs {
		require.Equal(t, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS, m.compactionJobQueue.jobs[job.Name].Status)
		require.Equal(t, uint64(20), m.compactionJobQueue.jobs[job.Name].RaftLogIndex)
		require.Equal(t, int64(15000000020), m.compactionJobQueue.jobs[job.Name].LeaseExpiresAt)
	}
	verifyCompactionState(t, m)

	// asking for more work results in 0 jobs
	respEmptyQueue, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 0, len(respEmptyQueue.CompactionJobs))
	verifyCompactionState(t, m)
}

func Test_StatusUpdates_Success(t *testing.T) {
	// add enough blocks to create 2 jobs
	m := initState(t)
	addLevel0Blocks(m, 40)
	require.Equal(t, 2, len(m.compactionJobQueue.jobs))

	// assign the 2 jobs
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 2}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.CompactionJobs))

	// mark the 2 jobs as completed with information about 2 compacted blocks
	statusUpdates := []*compactorv1.CompactionJobStatus{
		{
			JobName: resp.CompactionJobs[0].Name,
			Status:  compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS,
			CompletedJob: &compactorv1.CompletedJob{
				Blocks: []*metastorev1.BlockMeta{createBlock(40, 0, "", 1)},
			},
			RaftLogIndex: 20,
			Shard:        0,
			TenantId:     "",
		},
		{
			JobName: resp.CompactionJobs[1].Name,
			Status:  compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS,
			CompletedJob: &compactorv1.CompletedJob{
				Blocks: []*metastorev1.BlockMeta{createBlock(41, 0, "", 1)},
			},
			RaftLogIndex: 20,
			Shard:        0,
			TenantId:     "",
		},
	}
	_, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates}, 21, 21)
	require.NoError(t, err)
	verifyCompactionState(t, m)

	// completed jobs are removed from the queue
	require.Equalf(t, 0, len(m.compactionJobQueue.jobs), "compaction job queue should be empty")

	// compacted blocks are added
	b40 := m.getOrCreateShard(0).segments["b-40"]
	b41 := m.getOrCreateShard(0).segments["b-41"]
	require.NotNilf(t, b40, "compacted block not found in state")
	require.NotNilf(t, b41, "compacted block not found in state")
	require.Equalf(t, uint32(1), b40.CompactionLevel, "compacted block has wrong level")
	require.Equalf(t, uint32(1), b41.CompactionLevel, "compacted block has wrong level")

	// source blocks are removed
	for i := 0; i < 40; i++ {
		require.Nilf(t, m.getOrCreateShard(0).segments[fmt.Sprintf("b-%d", i)], "old block %d found in state", i)
	}
}

func Test_StatusUpdates_InProgress(t *testing.T) {
	// add blocks to create 1 job
	m := initState(t)
	addLevel0Blocks(m, 20)
	require.Equal(t, 1, len(m.compactionJobQueue.jobs))

	// assign the job to a worker
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.CompactionJobs))
	job := resp.CompactionJobs[0]
	require.Equal(t, int64(15000000020), m.compactionJobQueue.jobs[job.Name].LeaseExpiresAt)

	// send a "in progress" update from the worker
	statusUpdates := []*compactorv1.CompactionJobStatus{
		{
			JobName:      resp.CompactionJobs[0].Name,
			Status:       compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS,
			RaftLogIndex: 20,
			Shard:        0,
			TenantId:     "",
		},
	}
	_, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates}, 21, 21)
	require.NoError(t, err)
	verifyCompactionState(t, m)

	// verify that the job is still in progress and assigned to the same worker
	require.NotNil(t, m.compactionJobQueue.jobs[job.Name])
	require.Equalf(t, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS, m.compactionJobQueue.jobs[job.Name].Status, "status should be in progress")
	require.Equalf(t, int64(15000000021), m.compactionJobQueue.jobs[job.Name].LeaseExpiresAt, "the lease should be extended")
	require.Equal(t, uint64(20), m.compactionJobQueue.jobs[job.Name].RaftLogIndex)
}

func Test_OwnershipTransfer(t *testing.T) {
	// add blocks to create 1 job
	m := initState(t)
	addLevel0Blocks(m, 20)
	require.Equal(t, 1, len(m.compactionJobQueue.jobs))

	// assign the job to a worker
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.CompactionJobs))
	job := resp.CompactionJobs[0]
	require.Equal(t, int64(15000000020), m.compactionJobQueue.jobs[job.Name].LeaseExpiresAt)
	require.Equal(t, uint64(20), m.compactionJobQueue.jobs[job.Name].RaftLogIndex)

	// re-assign the job to a new worker when we are past the deadline
	resp, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 21, 15000000021)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.CompactionJobs))
	job = resp.CompactionJobs[0]
	require.Equal(t, int64(30000000021), m.compactionJobQueue.jobs[job.Name].LeaseExpiresAt)
	require.Equal(t, uint64(21), m.compactionJobQueue.jobs[job.Name].RaftLogIndex)
	verifyCompactionState(t, m)

	// reject a status update from the first worker
	statusUpdates := []*compactorv1.CompactionJobStatus{
		{
			JobName:      resp.CompactionJobs[0].Name,
			Status:       compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS,
			RaftLogIndex: 20,
			Shard:        0,
			TenantId:     "",
		},
	}
	_, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates}, 20, 20)
	require.NoError(t, err)
	require.NotNil(t, m.compactionJobQueue.jobs[job.Name])
	require.Equalf(t, compactionpb.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS, m.compactionJobQueue.jobs[job.Name].Status, "status should be in progress")

	// accept a status update from the second worker
	statusUpdates = []*compactorv1.CompactionJobStatus{
		{
			JobName: resp.CompactionJobs[0].Name,
			Status:  compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS,
			CompletedJob: &compactorv1.CompletedJob{
				Blocks: []*metastorev1.BlockMeta{createBlock(20, 0, "", 1)},
			},
			RaftLogIndex: 21,
			Shard:        0,
			TenantId:     "",
		},
	}
	_, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates}, 21, 30000000022)
	require.NoError(t, err)
	require.Nilf(t, m.compactionJobQueue.jobs[job.Name], "the job %s should be deleted", job.Name)
}

func Test_CompactedBlockCanCreateNewJob(t *testing.T) {
	// add 20 blocks to create a job
	m := initState(t)
	addLevel0Blocks(m, 20)

	// add 9 level 1 blocks so that we can create a job once a new level 1 block gets added (we need 10 blocks for level 1)
	addLevel1Blocks(m, "t1", 9)

	// assign the job to a worker
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.CompactionJobs))

	// complete the job with 2 compacted blocks
	statusUpdates := []*compactorv1.CompactionJobStatus{
		{
			JobName: resp.CompactionJobs[0].Name,
			Status:  compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS,
			CompletedJob: &compactorv1.CompletedJob{
				Blocks: []*metastorev1.BlockMeta{
					{
						Id:              "b-20-1",
						Shard:           uint32(0),
						TenantId:        "t1",
						CompactionLevel: uint32(1),
					},
					{
						Id:              "b-21-1",
						Shard:           uint32(0),
						TenantId:        "t1",
						CompactionLevel: uint32(1),
					},
				},
			},
			RaftLogIndex: 20,
			Shard:        0,
			TenantId:     "",
		},
	}
	resp, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates, JobCapacity: 1}, 20, 20)
	require.NoError(t, err)

	// the 9 original level-1 blocks and one of the new compacted blocks should form a new job
	require.Equalf(t, 1, len(m.compactionJobQueue.jobs), "there should be one job in the queue")
	job := resp.CompactionJobs[0]
	require.NotNilf(t, m.compactionJobQueue.jobs[job.Name].CompactionJob, "the job in the queue should be the returned one")

	// the second compacted block from the status update should be added to the block queue
	key := tenantShard{
		tenant: "t1",
		shard:  0,
	}
	require.Equalf(t, 1, len(m.compactionJobBlockQueues[key].blocksByLevel[1]), "there should be one level-1 block in the queue")
	require.Equalf(t, "b-21-1", m.compactionJobBlockQueues[key].blocksByLevel[1][0], "the block id should match the second compacted block")
}

func Test_FailedCompaction(t *testing.T) {
	m := initState(t)
	m.compactionConfig.JobMaxFailures = 2
	addLevel0Blocks(m, 20)

	// assign a job
	resp, err := m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	job := resp.CompactionJobs[0]

	// fail the job
	statusUpdates := []*compactorv1.CompactionJobStatus{
		{
			JobName:      job.Name,
			Status:       compactorv1.CompactionStatus_COMPACTION_STATUS_FAILURE,
			RaftLogIndex: 20,
		},
	}
	resp, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates, JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.NotNilf(t, m.compactionJobQueue.jobs[job.Name].CompactionJob, "the job %s should still exist", job.Name)
	require.Equalf(t, uint32(1), m.compactionJobQueue.jobs[job.Name].CompactionJob.Failures, "the job %s should have 1 failure", job.Name)
	require.Equalf(t, job.Name, resp.CompactionJobs[0].Name, "the job %s should be assigned again", job.Name)
	verifyCompactionState(t, m)

	// fail the job a second time, this time it will get marked as cancelled
	resp, err = m.pollCompactionJobs(&compactorv1.PollCompactionJobsRequest{JobStatusUpdates: statusUpdates, JobCapacity: 1}, 20, 20)
	require.NoError(t, err)
	require.Equalf(t, 0, len(resp.CompactionJobs), "no jobs should be left to assign")
	require.Equalf(t, compactionpb.CompactionStatus_COMPACTION_STATUS_CANCELLED, m.compactionJobQueue.jobs[job.Name].Status, "the job status should be cancelled")
	verifyCompactionState(t, m)
}

func addLevel0Blocks(m *metastoreState, count int) {
	for i := 0; i < count; i++ {
		b := createBlock(i, 0, "", 0)
		raftLog := &raft.Log{
			Index:      uint64(i),
			AppendedAt: time.Unix(0, int64(i)),
		}
		_, _ = m.applyAddBlock(raftLog, &metastorev1.AddBlockRequest{Block: b})
	}
}

func addLevel1Blocks(m *metastoreState, tenant string, count int) {
	for i := 0; i < count; i++ {
		b := createBlock(i, 0, tenant, 1)
		b.Id = fmt.Sprintf("b-%d-%d", i, 1)
		raftLog := &raft.Log{
			Index:      uint64(i),
			AppendedAt: time.Unix(0, int64(i)),
		}
		_, _ = m.applyAddBlock(raftLog, &metastorev1.AddBlockRequest{Block: b})
	}
}
