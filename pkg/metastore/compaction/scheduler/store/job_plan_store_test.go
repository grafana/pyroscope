package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/metastore/store"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestJobPlanStore(t *testing.T) {
	db := test.BoltDB(t)

	s := NewJobStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))
	assert.NoError(t, s.StoreJobPlan(tx, &raft_log.CompactionJobPlan{Name: "1"}))
	require.NoError(t, tx.Commit())

	s = NewJobStore()
	tx, err = db.Begin(false)
	require.NoError(t, err)
	state, err := s.GetJobPlan(tx, "2")
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, state)
	state, err = s.GetJobPlan(tx, "1")
	require.NoError(t, err)
	assert.Equal(t, "1", state.Name)
	require.NoError(t, tx.Rollback())

	tx, err = db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.DeleteJobPlan(tx, "1"))
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(false)
	require.NoError(t, err)
	state, err = s.GetJobPlan(tx, "1")
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, state)
	require.NoError(t, tx.Rollback())
}
