package metastore

import (
	"cmp"
	"context"
	"slices"

	"github.com/grafana/dskit/tracing"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
)

// ListCompactionJobs reads persisted job state without scheduling or claiming work.
func (svc *CompactionService) ListCompactionJobs(ctx context.Context, req *metastorev1.ListCompactionJobsRequest) (resp *metastorev1.ListCompactionJobsResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "CompactionService.ListCompactionJobs")
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()

	if tenant := req.GetTenant(); tenant != "" {
		span.SetTag("tenant_id", tenant)
	}

	resp = &metastorev1.ListCompactionJobsResponse{}
	var readErr error
	err = svc.state.ConsistentRead(ctx, func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		store := scheduler.NewStore()
		entries := store.ListEntries(tx)
		defer entries.Close()
		for entries.Next() {
			if readErr = ctx.Err(); readErr != nil {
				return
			}
			state := entries.At()
			plan, err := store.GetJobPlan(tx, state.Name)
			if err != nil {
				readErr = err
				return
			}
			if req.Tenant != "" && plan.Tenant != req.Tenant {
				continue
			}
			resp.Jobs = append(resp.Jobs, &metastorev1.CompactionJobInfo{
				Job:    &metastorev1.CompactionJob{Name: plan.Name, Tenant: plan.Tenant, Shard: plan.Shard, CompactionLevel: plan.CompactionLevel, SourceBlocks: plan.SourceBlocks, Tombstones: plan.Tombstones, DeleteFilter: plan.DeleteFilter},
				Status: state.Status, AddedAt: state.AddedAt, LeaseExpiresAt: state.LeaseExpiresAt, Failures: state.Failures,
			})
		}
		readErr = entries.Err()
	})
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if readErr != nil {
		return nil, readErr
	}
	slices.SortFunc(resp.Jobs, func(a, b *metastorev1.CompactionJobInfo) int {
		if n := cmp.Compare(a.Job.CompactionLevel, b.Job.CompactionLevel); n != 0 {
			return n
		}
		if n := cmp.Compare(a.AddedAt, b.AddedAt); n != 0 {
			return n
		}
		return cmp.Compare(a.Job.Name, b.Job.Name)
	})
	span.SetTag("job_count", len(resp.Jobs))
	return resp, nil
}
