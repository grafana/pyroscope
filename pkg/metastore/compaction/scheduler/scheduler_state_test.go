package scheduler

import (
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
		jobs, err := scheduler.ListJobs(tx)
		require.NoError(t, err)
		require.Len(t, jobs, 2)

		assert.Equal(t, "job-a", jobs[0].State.Name)
		assert.Equal(t, uint64(42), jobs[0].State.Token)
		assert.Equal(t, "tenant-a", jobs[0].Tenant)
		assert.Equal(t, uint32(1), jobs[0].Shard)
		assert.Equal(t, uint32(2), jobs[0].SourceBlocks)

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
		jobs, err := scheduler.ListJobs(tx)
		require.NoError(t, err)
		assert.Empty(t, jobs)
		return nil
	}))
}
