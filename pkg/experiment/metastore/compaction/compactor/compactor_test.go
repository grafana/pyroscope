package compactor

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction/compactor/store"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockcompactor"
)

func TestCompactor_AddBlock(t *testing.T) {
	queueStore := new(mockcompactor.MockBlockQueueStore)
	tombstones := new(mockcompactor.MockTombstones)

	md := &metastorev1.BlockMeta{TenantId: "A", Shard: 0, CompactionLevel: 0, Id: "1"}
	cmd := &raft.Log{Index: uint64(1), AppendedAt: time.Unix(0, 0)}
	compactor := NewCompactor(testConfig, queueStore, tombstones)

	testErr := errors.New("x")
	t.Run("fails if cannot store the entry", assertIdempotentSubtest(t, func(t *testing.T) {
		queueStore.On("StoreEntry", mock.Anything, mock.Anything).Return(testErr)
		require.ErrorIs(t, compactor.AddBlock(nil, cmd, md), testErr)
	}))

	queueStore.AssertExpectations(t)
	tombstones.AssertExpectations(t)
}

func TestCompactor_UpdatePlan(t *testing.T) {
	const N = 10

	tombstones := new(mockcompactor.MockTombstones)
	queueStore := new(mockcompactor.MockBlockQueueStore)
	queueStore.On("StoreEntry", mock.Anything, mock.Anything).
		Return(nil).Times(N)

	compactor := NewCompactor(testConfig, queueStore, tombstones)
	now := time.Unix(0, 0)
	for i := 0; i < N; i++ {
		cmd := &raft.Log{Index: uint64(1), AppendedAt: now}
		md := &metastorev1.BlockMeta{TenantId: "A", Shard: 0, CompactionLevel: 0, Id: strconv.Itoa(i)}
		err := compactor.AddBlock(nil, cmd, md)
		require.NoError(t, err)
	}

	planned := make([]*raft_log.CompactionJobPlan, 3)
	assertIdempotent(t, func(t *testing.T) {
		tombstones.On("ListTombstones", mock.Anything).
			Return(iter.NewEmptyIterator[*metastorev1.Tombstones](), nil)

		planner := compactor.NewPlan(nil, &raft.Log{Index: uint64(2), AppendedAt: now})
		for i := range planned {
			job, err := planner.CreateJob()
			require.NoError(t, err)
			require.NotNil(t, job)
			planned[i] = job
		}

		job, err := planner.CreateJob()
		require.NoError(t, err)
		require.Nil(t, job)
	})

	// UpdatePlan is mostly idempotent, except it won't
	// DeleteEntry that is not loaded into memory.
	queueStore.On("DeleteEntry", mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Times(9)

	assertIdempotent(t, func(t *testing.T) {
		newJobs := make([]*raft_log.CompactionJobUpdate, 3)
		for i := range planned {
			newJobs[i] = &raft_log.CompactionJobUpdate{Plan: planned[i]}
		}

		update := &raft_log.CompactionPlanUpdate{NewJobs: newJobs}
		cmd := &raft.Log{Index: uint64(2), AppendedAt: now}
		require.NoError(t, compactor.UpdatePlan(nil, cmd, update))

		planner := compactor.NewPlan(nil, &raft.Log{Index: uint64(3), AppendedAt: now})
		job, err := planner.CreateJob()
		require.NoError(t, err)
		require.Nil(t, job)
	})

	queueStore.AssertExpectations(t)
	tombstones.AssertExpectations(t)
}

func TestCompactor_Restore(t *testing.T) {
	queueStore := new(mockcompactor.MockBlockQueueStore)
	queueStore.On("ListEntries", mock.Anything).Return(iter.NewSliceIterator([]store.BlockEntry{
		{Index: 0, ID: "0", Tenant: "A"},
		{Index: 1, ID: "1", Tenant: "A"},
		{Index: 2, ID: "2", Tenant: "A"},
		{Index: 3, ID: "3", Tenant: "A"},
	}))

	tombstones := new(mockcompactor.MockTombstones)
	tombstones.On("ListTombstones", mock.Anything).
		Return(iter.NewEmptyIterator[*metastorev1.Tombstones](), nil)

	compactor := NewCompactor(testConfig, queueStore, tombstones)
	require.NoError(t, compactor.Restore(nil))

	planner := compactor.NewPlan(nil, new(raft.Log))
	planned, err := planner.CreateJob()
	require.NoError(t, err)
	require.NotEmpty(t, planned)
}
