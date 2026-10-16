package metastore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/grafana/dskit/tracing"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
	metastoretracing "github.com/grafana/pyroscope/v2/pkg/metastore/tracing"
)

type deletionIndex interface {
	DeletionCandidates(context.Context, *bbolt.Tx, string, int64, int64) ([]*metastorev1.BlockMeta, error)
	GetBlocks(*bbolt.Tx, *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error)
}

type deletionScheduler interface {
	BusyBlocks(*bbolt.Tx) ([]*metastorev1.BlockList, error)
}

type deletionBlockKey struct {
	tenant string
	shard  uint32
	id     string
}

type datasetDeletionRejected struct {
	reason  raft_log.DatasetDeletionRejection_Reason
	message string
}

func (e *datasetDeletionRejected) Error() string { return e.message }

func rejectDatasetDeletion(reason raft_log.DatasetDeletionRejection_Reason, message string) error {
	return &datasetDeletionRejected{reason: reason, message: message}
}

func deletionRPCError(err error) error {
	var rejection *datasetDeletionRejected
	if errors.As(err, &rejection) {
		return deletionRejectionStatus(&raft_log.DatasetDeletionRejection{Reason: rejection.reason, Message: rejection.message})
	}
	return err
}

func deletionRejectionStatus(rejection *raft_log.DatasetDeletionRejection) error {
	code := codes.Internal
	switch rejection.Reason {
	case raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST:
		code = codes.InvalidArgument
	case raft_log.DatasetDeletionRejection_REASON_UNSUPPORTED:
		code = codes.Unimplemented
	case raft_log.DatasetDeletionRejection_REASON_BLOCK_BUSY:
		code = codes.FailedPrecondition
	case raft_log.DatasetDeletionRejection_REASON_SCHEDULER_FULL:
		code = codes.ResourceExhausted
	}
	return status.Error(code, rejection.Message)
}

func validateDeletionRequest(req *metastorev1.DeleteDatasetsRequest) (*regexp.Regexp, error) {
	if req == nil || strings.TrimSpace(req.Tenant) == "" {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, "tenant is required")
	}
	if req.Filter == nil {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, "filter is required")
	}
	if req.MinTime > req.MaxTime {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, "min_time exceeds max_time")
	}
	re, err := block.CompileServiceNameRegex(req.GetFilter().GetServiceNameRegex())
	if err != nil {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, err.Error())
	}
	return re, nil
}

// DeleteDatasets plans full-dataset removal, never sample-level time filtering.
// Dry runs use a consistent read and do not append anything to the Raft log.
func (svc *CompactionService) DeleteDatasets(ctx context.Context, req *metastorev1.DeleteDatasetsRequest) (resp *metastorev1.DeleteDatasetsResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "CompactionService.DeleteDatasets")
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()

	span.SetTag("tenant_id", req.GetTenant())
	span.SetTag("dry_run", req.GetDryRun())
	span.SetTag("min_time", req.GetMinTime())
	span.SetTag("max_time", req.GetMaxTime())
	span.SetTag("min_compaction_level", req.GetMinCompactionLevel())

	if _, err = validateDeletionRequest(req); err != nil {
		return nil, deletionRPCError(err)
	}
	if req.DryRun {
		// Dry runs are plain consistent reads: they neither propose anything
		// nor depend on the prepare/update sequence, so they must not block
		// worker polling behind svc.mu.
		readErr := svc.state.ConsistentRead(ctx, func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
			resp, err = svc.deletionHandler.datasetDeletionReport(ctx, tx, req)
		})
		if readErr != nil {
			return nil, status.Error(codes.Unavailable, readErr.Error())
		}
		if err != nil {
			return nil, deletionRPCError(err)
		}
		span.SetTag("matched_blocks", resp.GetMatchedBlocksCount())
		return resp, nil
	}
	// The prepare/update pair below must not interleave with another plan
	// update (PollCompactionJobs or a concurrent deletion), same as polling.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cmd := fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_PREPARE_DATASET_DELETION)
	result, err := svc.raft.Propose(ctx, cmd, datasetDeletionCommand(req))
	if err != nil {
		return nil, err
	}
	prepared := result.(*raft_log.PrepareDatasetDeletionResponse)
	if rejection := prepared.GetRejection(); rejection != nil {
		return nil, deletionRejectionStatus(rejection)
	}
	if len(prepared.PlanUpdate.NewJobs) == 0 {
		resp = prepared.GetDeletion()
		span.SetTag("matched_blocks", resp.GetMatchedBlocksCount())
		span.SetTag("scheduled_jobs", len(resp.GetScheduledJobNames()))
		return resp, nil
	}
	cmd = fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN)
	result, err = svc.raft.Propose(ctx, cmd, &raft_log.UpdateCompactionPlanRequest{Term: prepared.Term, PlanUpdate: prepared.PlanUpdate})
	if err != nil {
		return nil, err
	}
	if result.(*raft_log.UpdateCompactionPlanResponse).PlanUpdate == nil {
		return nil, status.Error(codes.FailedPrecondition, "deletion plan changed; retry request")
	}
	resp = prepared.GetDeletion()
	span.SetTag("matched_blocks", resp.GetMatchedBlocksCount())
	span.SetTag("scheduled_jobs", len(resp.GetScheduledJobNames()))
	return resp, nil
}

func (h *CompactionCommandHandler) datasetDeletionReport(ctx context.Context, tx *bbolt.Tx, req *metastorev1.DeleteDatasetsRequest) (*metastorev1.DeleteDatasetsResponse, error) {
	re, err := validateDeletionRequest(req)
	if err != nil {
		return nil, err
	}
	idx, ok := h.index.(deletionIndex)
	if !ok {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_UNSUPPORTED, "index does not support dataset deletion")
	}
	blocks, err := idx.DeletionCandidates(ctx, tx, req.Tenant, req.MinTime, req.MaxTime)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(blocks, func(a, b *metastorev1.BlockMeta) int { return strings.Compare(a.Id, b.Id) })
	resp := &metastorev1.DeleteDatasetsResponse{DryRun: req.DryRun}
	for _, b := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if b.CompactionLevel == 0 || b.Tenant == 0 || b.CompactionLevel < req.MinCompactionLevel {
			continue
		}
		owner := metadata.Tenant(b)
		if owner != req.Tenant || h.tombstones.Exists(owner, b.Shard, b.Id) {
			continue
		}
		s := &metastorev1.BlockDeletionSummary{BlockId: b.Id, Tenant: owner, Shard: b.Shard, CompactionLevel: b.CompactionLevel, BlockMinTime: time.UnixMilli(b.MinTime).UnixNano(), BlockMaxTime: time.UnixMilli(b.MaxTime).UnixNano()}
		for _, ds := range b.Datasets {
			if ds.Name == 0 || ds.Format != 0 {
				continue
			}
			if b.StringTable[ds.Tenant] != req.Tenant || !re.MatchString(b.StringTable[ds.Name]) {
				s.RemainingDatasetsCount++
				continue
			}
			// Block metadata uses milliseconds; the deletion API uses nanoseconds.
			minTime, maxTime := time.UnixMilli(ds.MinTime).UnixNano(), time.UnixMilli(ds.MaxTime).UnixNano()
			if s.MatchedDatasetsCount == 0 {
				s.MatchedMinTime, s.MatchedMaxTime = minTime, maxTime
			} else {
				s.MatchedMinTime = min(s.MatchedMinTime, minTime)
				s.MatchedMaxTime = max(s.MatchedMaxTime, maxTime)
			}
			s.MatchedDatasetsCount++
			s.MatchedBytes += ds.Size
			s.MatchedServices = append(s.MatchedServices, b.StringTable[ds.Name])
		}
		if s.MatchedDatasetsCount == 0 {
			continue
		}
		slices.Sort(s.MatchedServices)
		s.MatchedServices = slices.Compact(s.MatchedServices)
		s.FullBlockPlanned = s.RemainingDatasetsCount == 0
		if s.FullBlockPlanned {
			resp.PlannedFullBlockDeletionsCount++
		} else {
			resp.PlannedBlockRewritesCount++
		}
		if resp.MatchedBlocksCount == 0 {
			resp.EffectiveMatchedMinTime, resp.EffectiveMatchedMaxTime = s.MatchedMinTime, s.MatchedMaxTime
		} else {
			resp.EffectiveMatchedMinTime = min(resp.EffectiveMatchedMinTime, s.MatchedMinTime)
			resp.EffectiveMatchedMaxTime = max(resp.EffectiveMatchedMaxTime, s.MatchedMaxTime)
		}
		resp.MatchedBlocksCount++
		resp.MatchedDatasetsCount += s.MatchedDatasetsCount
		resp.MatchedBytes += s.MatchedBytes
		resp.Blocks = append(resp.Blocks, s)
	}
	return resp, nil
}

func (h *CompactionCommandHandler) busyDeletionBlocks(tx *bbolt.Tx) (map[deletionBlockKey]bool, error) {
	sc, ok := h.scheduler.(deletionScheduler)
	if !ok {
		return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_UNSUPPORTED, "scheduler does not support dataset deletion")
	}
	lists, err := sc.BusyBlocks(tx)
	if err != nil {
		return nil, err
	}
	busy := make(map[deletionBlockKey]bool)
	for _, list := range lists {
		for _, id := range list.Blocks {
			busy[deletionBlockKey{list.Tenant, list.Shard, id}] = true
		}
	}
	return busy, nil
}

func datasetDeletionCommand(req *metastorev1.DeleteDatasetsRequest) *raft_log.PrepareDatasetDeletionRequest {
	return &raft_log.PrepareDatasetDeletionRequest{
		Tenant: req.Tenant, MinTime: req.MinTime, MaxTime: req.MaxTime,
		Filter: req.Filter, MinCompactionLevel: req.MinCompactionLevel,
	}
}

// PrepareDatasetDeletion plans deletion independently of worker polling.
// An expected rejection must not fail the FSM transaction or poison replay.
func (h *CompactionCommandHandler) PrepareDatasetDeletion(ctx context.Context, tx *bbolt.Tx, cmd *raft.Log, req *raft_log.PrepareDatasetDeletionRequest) (resp *raft_log.PrepareDatasetDeletionResponse, err error) {
	span, ctx := metastoretracing.StartSpanFromContext(ctx, "raft.PrepareDatasetDeletion")
	span.SetTag("tenant_id", req.GetTenant())
	span.SetTag("raft_log_index", cmd.Index)
	span.SetTag("raft_log_term", cmd.Term)
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()

	prepared, err := h.prepareDatasetDeletion(ctx, tx, cmd, &metastorev1.DeleteDatasetsRequest{
		Tenant: req.Tenant, MinTime: req.MinTime, MaxTime: req.MaxTime,
		Filter: req.Filter, MinCompactionLevel: req.MinCompactionLevel,
	})
	if err == nil {
		span.SetTag("new_jobs", len(prepared.GetPlanUpdate().GetNewJobs()))
		return prepared, nil
	}
	var rejection *datasetDeletionRejected
	if !errors.As(err, &rejection) {
		return nil, err
	}
	span.SetTag("rejected_reason", rejection.reason.String())
	return &raft_log.PrepareDatasetDeletionResponse{Term: cmd.Term, Outcome: &raft_log.PrepareDatasetDeletionResponse_Rejection{
		Rejection: &raft_log.DatasetDeletionRejection{Reason: rejection.reason, Message: rejection.message},
	}}, nil
}

func (h *CompactionCommandHandler) prepareDatasetDeletion(ctx context.Context, tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.DeleteDatasetsRequest) (*raft_log.PrepareDatasetDeletionResponse, error) {
	resp, err := h.datasetDeletionReport(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	update := new(raft_log.CompactionPlanUpdate)
	if req.DryRun {
		return &raft_log.PrepareDatasetDeletionResponse{Term: cmd.Term, PlanUpdate: update, Outcome: &raft_log.PrepareDatasetDeletionResponse_Deletion{Deletion: resp}}, nil
	}
	busy, err := h.busyDeletionBlocks(tx)
	if err != nil {
		return nil, err
	}
	schedule := h.scheduler.NewSchedule(tx, cmd)
	for _, b := range resp.Blocks {
		if busy[deletionBlockKey{b.Tenant, b.Shard, b.BlockId}] {
			return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_BLOCK_BUSY, fmt.Sprintf("block %s already has a compaction job; retry after completion", b.BlockId))
		}
		hash := sha256.Sum256([]byte(fmt.Sprintf("%q/%d/%s/%q", b.Tenant, b.Shard, b.BlockId, req.GetFilter().GetServiceNameRegex())))
		plan := &raft_log.CompactionJobPlan{
			Name:   fmt.Sprintf("%x-DELETE-S%d-L%d", hash[:16], b.Shard, b.CompactionLevel),
			Tenant: b.Tenant, Shard: b.Shard, CompactionLevel: b.CompactionLevel, SourceBlocks: []string{b.BlockId},
			DeleteFilter: &metastorev1.DeleteFilter{ServiceNameRegex: req.GetFilter().GetServiceNameRegex()},
		}
		// Busy blocks were rejected above, so a nil state can only mean the
		// queue is full. Failed jobs are evicted by the regular polling path.
		state := schedule.AddJob(plan)
		if state == nil {
			return nil, rejectDatasetDeletion(raft_log.DatasetDeletionRejection_REASON_SCHEDULER_FULL, "compaction scheduler is full; no deletion jobs scheduled")
		}
		update.NewJobs = append(update.NewJobs, &raft_log.NewCompactionJob{Plan: plan, State: state})
		resp.ScheduledJobNames = append(resp.ScheduledJobNames, plan.Name)
	}
	return &raft_log.PrepareDatasetDeletionResponse{Term: cmd.Term, PlanUpdate: update, Outcome: &raft_log.PrepareDatasetDeletionResponse_Deletion{Deletion: resp}}, nil
}

func (h *CompactionCommandHandler) validateDeletionJobs(ctx context.Context, tx *bbolt.Tx, update *raft_log.CompactionPlanUpdate) (bool, error) {
	var busy map[deletionBlockKey]bool
	for _, j := range update.NewJobs {
		if j.Plan.DeleteFilter == nil {
			continue
		}
		if busy == nil {
			var err error
			busy, err = h.busyDeletionBlocks(tx)
			if err != nil {
				return false, err
			}
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		idx, ok := h.index.(deletionIndex)
		if !ok {
			return false, nil
		}
		p := j.Plan
		for _, id := range p.SourceBlocks {
			key := deletionBlockKey{p.Tenant, p.Shard, id}
			if busy[key] || h.tombstones.Exists(p.Tenant, p.Shard, id) {
				return false, nil
			}
			busy[key] = true
		}
		blocks, err := idx.GetBlocks(tx, &metastorev1.BlockList{Tenant: p.Tenant, Shard: p.Shard, Blocks: p.SourceBlocks})
		if err != nil {
			return false, err
		}
		if len(blocks) != len(p.SourceBlocks) {
			return false, nil
		}
	}
	return true, nil
}
