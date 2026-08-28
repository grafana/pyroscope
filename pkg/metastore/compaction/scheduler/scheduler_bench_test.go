package scheduler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

const (
	// benchmarkJobs matches the default compaction-max-job-queue-size: the
	// scheduler refuses to add jobs beyond it, so this is the worst case.
	benchmarkJobs = 10_000
	// benchmarkSourceBlocks is a typical job width.
	benchmarkSourceBlocks = 20
)

func benchmarkBlockIDs(n int) []string {
	blocks := make([]string, n)
	for i := range blocks {
		blocks[i] = fmt.Sprintf("01M14VWWJJEDW4TPDEFH%06d", i)
	}
	return blocks
}

// benchmarkSchedule fills the schedule with the maximum number of jobs. Each
// plan carries the source block list, and optionally a tombstone list, which
// the listing decodes and discards.
func benchmarkSchedule(tb testing.TB, tombstones int) (*bbolt.DB, *Scheduler) {
	db := test.BoltDB(tb)
	jobStore := NewStore()
	sc := NewScheduler(Config{MaxQueueSize: benchmarkJobs}, jobStore, nil)
	require.NoError(tb, db.Update(func(tx *bbolt.Tx) error {
		if err := jobStore.CreateBuckets(tx); err != nil {
			return err
		}
		for i := 0; i < benchmarkJobs; i++ {
			name := fmt.Sprintf("%016x-T-tenant-%04d-S%d-L1", i, i%1000, i%64)
			if err := jobStore.StoreJobState(tx, &raft_log.CompactionJobState{
				Name:            name,
				CompactionLevel: 1,
				AddedAt:         int64(i),
			}); err != nil {
				return err
			}
			plan := &raft_log.CompactionJobPlan{
				Name:            name,
				Tenant:          fmt.Sprintf("tenant-%04d", i%1000),
				Shard:           uint32(i % 64),
				CompactionLevel: 1,
				SourceBlocks:    benchmarkBlockIDs(benchmarkSourceBlocks),
			}
			if tombstones > 0 {
				plan.Tombstones = []*metastorev1.Tombstones{{
					Blocks: &metastorev1.BlockTombstones{
						Name:   name,
						Tenant: plan.Tenant,
						Shard:  plan.Shard,
						Blocks: benchmarkBlockIDs(tombstones),
					},
				}}
			}
			if err := jobStore.StoreJobPlan(tx, plan); err != nil {
				return err
			}
		}
		return nil
	}))
	return db, sc
}

// BenchmarkScheduler_ListJobs measures the job scan that every compaction page
// load performs. Every plan is read and decoded regardless of the filter, as
// the tenant of a job is part of its plan rather than its state.
func BenchmarkScheduler_ListJobs(b *testing.B) {
	tenant := "tenant-0005"
	for _, test := range []struct {
		name   string
		filter JobFilter
	}{
		{name: "unfiltered", filter: JobFilter{}},
		{name: "tenant", filter: JobFilter{Tenant: &tenant}},
		{name: "tenant_with_source_blocks", filter: JobFilter{Tenant: &tenant, IncludeSourceBlocks: true}},
		{name: "all_with_source_blocks", filter: JobFilter{IncludeSourceBlocks: true}},
	} {
		b.Run(test.name, func(b *testing.B) {
			db, sc := benchmarkSchedule(b, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				require.NoError(b, db.View(func(tx *bbolt.Tx) error {
					jobs, err := sc.ListJobs(tx, test.filter)
					require.NoError(b, err)
					require.NotEmpty(b, jobs)
					return nil
				}))
			}
		})
	}
}

// BenchmarkScheduler_ListJobs_tombstones measures the cost of the tombstone
// lists carried by the job plans. The listing never reads them.
func BenchmarkScheduler_ListJobs_tombstones(b *testing.B) {
	for _, n := range []int{0, 20, 200} {
		b.Run(fmt.Sprintf("tombstones=%d", n), func(b *testing.B) {
			db, sc := benchmarkSchedule(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				require.NoError(b, db.View(func(tx *bbolt.Tx) error {
					_, err := sc.ListJobs(tx, JobFilter{})
					return err
				}))
			}
		})
	}
}
