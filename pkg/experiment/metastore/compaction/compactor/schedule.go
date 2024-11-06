package compactor

import (
	"time"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

type compactionSchedule struct {
	tx   *bbolt.Tx
	raft *raft.Log

	scheduler *Scheduler
	store     JobStore

	// TODO(kolesnikovae): Configuration.
	lease       time.Duration
	maxFailures int64
}

func (p *compactionSchedule) AssignJob() (*metastorev1.CompactionJob, *raft_log.CompactionJobState, error) {
	return nil, nil, nil
}

func (p *compactionSchedule) UpdateJob(status *metastorev1.CompactionJobStatusUpdate) (*raft_log.CompactionJobState, error) {
	state, err := p.store.GetJobState(p.tx, status.Name)
	if err != nil {
		return nil, err
	}
	if state == nil {
		// The job is not found. This may happen if the job has been
		// reassigned and completed by another worker.
		return nil, nil
	}
	if state.RaftLogIndex > status.Token {
		// The job is not assigned to this worker.
		return nil, nil
	}

	switch status.Status {
	default:
		// Not allowed and unknown status updates are ignored: eventually,
		// the job will be reassigned. The same for status handlers: a nil
		// state is returned, which is interpreted as "no new lease":
		// stop the work.
		return nil, nil

	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		return p.handleInProgress(state), nil
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
		return p.handleSuccess(state, status)
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_FAILURE:
		return p.handleFailure(state), nil
	}
}

func (p *compactionSchedule) handleSuccess(
	state *raft_log.CompactionJobState,
	update *metastorev1.CompactionJobStatusUpdate,
) (*raft_log.CompactionJobState, error) {
	source, err := p.store.GetSourceBlocks(p.tx, state.Name)
	if err != nil {
		return nil, err
	}
	updated := &raft_log.CompactionJobState{
		Name:            state.Name,
		CompactionLevel: state.CompactionLevel,
		Status:          update.Status,
		RaftLogIndex:    update.Token,
		LeaseExpiresAt:  state.LeaseExpiresAt,
		Failures:        state.Failures,
		AddedAt:         state.AddedAt,
		CompactedBlocks: &raft_log.CompactedBlocks{
			JobName:         state.Name,
			Tenant:          "", // TODO: Include in status.
			Shard:           0,
			CompactionLevel: 0,
			SourceBlocks:    source,
			CompactedBlocks: update.CompactedBlocks,
			DeletedBlocks:   update.DeletedBlocks,
		},
	}
	return updated, nil
}

func (p *compactionSchedule) handleInProgress(state *raft_log.CompactionJobState) *raft_log.CompactionJobState {
	return &raft_log.CompactionJobState{
		Name:            state.Name,
		CompactionLevel: state.CompactionLevel,
		Status:          state.Status,
		RaftLogIndex:    state.RaftLogIndex,
		LeaseExpiresAt:  p.raft.AppendedAt.Add(p.lease).UnixNano(),
		Failures:        state.Failures,
		AddedAt:         state.AddedAt,
	}
}

func (p *compactionSchedule) handleFailure(state *raft_log.CompactionJobState) *raft_log.CompactionJobState {
	status := metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
	failures := state.Failures
	if failures++; failures >= p.maxFailures {
		status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_CANCELLED
	}
	return &raft_log.CompactionJobState{
		Name:            state.Name,
		CompactionLevel: state.CompactionLevel,
		Status:          status,
		RaftLogIndex:    p.raft.Index,
		LeaseExpiresAt:  state.LeaseExpiresAt,
		Failures:        failures,
		AddedAt:         state.AddedAt,
	}
}
