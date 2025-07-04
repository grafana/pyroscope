package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/metastore/store"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestJobStateStore(t *testing.T) {
	db := test.BoltDB(t)

	s := NewJobStore()
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, s.CreateBuckets(tx))
	assert.NoError(t, s.StoreJobState(tx, &raft_log.CompactionJobState{Name: "1"}))
	assert.NoError(t, s.StoreJobState(tx, &raft_log.CompactionJobState{Name: "2"}))
	assert.NoError(t, s.StoreJobState(tx, &raft_log.CompactionJobState{Name: "3"}))
	require.NoError(t, tx.Commit())

	s = NewJobStore()
	tx, err = db.Begin(true)
	require.NoError(t, err)
	state, err := s.GetJobState(tx, "2")
	require.NoError(t, err)
	assert.Equal(t, "2", state.Name)
	require.NoError(t, s.DeleteJobState(tx, "2"))
	state, err = s.GetJobState(tx, "2")
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, state)
	require.NoError(t, tx.Commit())

	tx, err = db.Begin(true)
	require.NoError(t, err)

	iter := s.ListEntries(tx)
	expected := []string{"1", "3"}
	var i int
	for iter.Next() {
		assert.Equal(t, expected[i], iter.At().Name)
		i++
	}
	assert.Nil(t, iter.Err())
	assert.Nil(t, iter.Close())
	require.NoError(t, tx.Rollback())
}
