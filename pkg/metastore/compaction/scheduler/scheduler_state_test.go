package scheduler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestScheduler_ListJobs(t *testing.T) {
	db := test.BoltDB(t)
	store := NewStore()
	scheduler := NewScheduler(Config{}, store, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, store.CreateBuckets(tx))
		require.NoError(t, store.StoreJobState(tx, &raft_log.CompactionJobState{
			Name:            "job-a",
			CompactionLevel: 0,
			Token:           42,
		}))
		require.NoError(t, store.StoreJobPlan(tx, &raft_log.CompactionJobPlan{
			Name:            "job-a",
			Tenant:          "tenant-a",
			Shard:           1,
			CompactionLevel: 0,
			SourceBlocks:    []string{"b1", "b2"},
		}))
		// A job state without a plan must be tolerated.
		require.NoError(t, store.StoreJobState(tx, &raft_log.CompactionJobState{
			Name:            "job-b",
			CompactionLevel: 1,
		}))
		return nil
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		jobs, err := scheduler.ListJobs(context.Background(), tx, JobFilter{})
		require.NoError(t, err)
		require.Len(t, jobs, 2)

		assert.Equal(t, "job-a", jobs[0].State.Name)
		assert.Equal(t, uint64(42), jobs[0].State.Token)
		assert.Equal(t, "tenant-a", jobs[0].Tenant)
		assert.Equal(t, uint32(1), jobs[0].Shard)
		assert.Equal(t, uint32(2), jobs[0].SourceBlocks)
		// The source block list is not retained unless it is requested.
		assert.Nil(t, jobs[0].SourceBlockIDs)

		assert.Equal(t, "job-b", jobs[1].State.Name)
		assert.Empty(t, jobs[1].Tenant)
		assert.Zero(t, jobs[1].SourceBlocks)
		return nil
	}))
}

func TestScheduler_ListJobs_empty(t *testing.T) {
	db := test.BoltDB(t)
	store := NewStore()
	scheduler := NewScheduler(Config{}, store, nil)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return store.CreateBuckets(tx)
	}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		jobs, err := scheduler.ListJobs(context.Background(), tx, JobFilter{})
		require.NoError(t, err)
		assert.Empty(t, jobs)
		return nil
	}))
}

func TestScheduler_ListJobs_canceled(t *testing.T) {
	db := test.BoltDB(t)
	store := NewStore()
	scheduler := NewScheduler(Config{}, store, nil)

	// More jobs than the cancellation check interval, so the scan reaches it.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, store.CreateBuckets(tx))
		for i := 0; i < jobScanCancelInterval*2; i++ {
			name := fmt.Sprintf("job-%06d", i)
			require.NoError(t, store.StoreJobState(tx, &raft_log.CompactionJobState{Name: name}))
			require.NoError(t, store.StoreJobPlan(tx, &raft_log.CompactionJobPlan{
				Name:   name,
				Tenant: "tenant-a",
			}))
		}
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		_, err := scheduler.ListJobs(ctx, tx, JobFilter{})
		require.ErrorIs(t, err, context.Canceled)
		return nil
	}))
}

func TestScheduler_ListJobs_filter(t *testing.T) {
	db := test.BoltDB(t)
	store := NewStore()
	scheduler := NewScheduler(Config{}, store, nil)

	// A job of a named tenant, a job of no tenant at all (level 0 compacts
	// the multi-tenant segments), and a job state whose plan is missing.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		require.NoError(t, store.CreateBuckets(tx))
		for _, job := range []*raft_log.CompactionJobPlan{
			{Name: "job-a", Tenant: "tenant-a", Shard: 1, SourceBlocks: []string{"b1", "b2"}},
			{Name: "job-s", Tenant: "", Shard: 2, SourceBlocks: []string{"b3"}},
		} {
			require.NoError(t, store.StoreJobState(tx, &raft_log.CompactionJobState{Name: job.Name}))
			require.NoError(t, store.StoreJobPlan(tx, job))
		}
		require.NoError(t, store.StoreJobState(tx, &raft_log.CompactionJobState{Name: "job-x"}))
		return nil
	}))

	names := func(t *testing.T, filter JobFilter) []string {
		var found []string
		require.NoError(t, db.View(func(tx *bbolt.Tx) error {
			jobs, err := scheduler.ListJobs(context.Background(), tx, filter)
			require.NoError(t, err)
			for _, job := range jobs {
				found = append(found, job.State.Name)
			}
			return nil
		}))
		return found
	}

	tenantA := "tenant-a"
	noTenant := ""

	// No filter lists everything, including the job without a plan.
	assert.Equal(t, []string{"job-a", "job-s", "job-x"}, names(t, JobFilter{}))
	// A named tenant selects only its own jobs.
	assert.Equal(t, []string{"job-a"}, names(t, JobFilter{Tenant: &tenantA}))
	// The empty tenant selects the jobs that have no tenant, and excludes
	// the job whose tenant cannot be determined.
	assert.Equal(t, []string{"job-s"}, names(t, JobFilter{Tenant: &noTenant}))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		jobs, err := scheduler.ListJobs(context.Background(), tx, JobFilter{
			Tenant:              &tenantA,
			IncludeSourceBlocks: true,
		})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, []string{"b1", "b2"}, jobs[0].SourceBlockIDs)
		// The count is reported either way.
		assert.Equal(t, uint32(2), jobs[0].SourceBlocks)
		return nil
	}))
}
