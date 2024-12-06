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
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockcompactor"
)

func TestCompactor_Compact(t *testing.T) {
	queueStore := new(mockcompactor.MockBlockQueueStore)
	tombstones := new(mockcompactor.MockTombstones)

	e := compaction.BlockEntry{
		Index:      1,
		AppendedAt: time.Unix(0, 0).UnixNano(),
		ID:         "1",
		Tenant:     "A",
	}

	compactor := NewCompactor(testConfig, queueStore, tombstones, nil)
	testErr := errors.New("x")
	t.Run("fails if cannot store the entry", test.AssertIdempotentSubtest(t, func(t *testing.T) {
		queueStore.On("StoreEntry", mock.Anything, mock.Anything).Return(testErr)
		require.ErrorIs(t, compactor.Compact(nil, e), testErr)
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

	compactor := NewCompactor(testConfig, queueStore, tombstones, nil)
	now := time.Unix(0, 0)
	for i := 0; i < N; i++ {
		err := compactor.Compact(nil, compaction.BlockEntry{
			Index:      1,
			AppendedAt: now.UnixNano(),
			ID:         strconv.Itoa(i),
			Tenant:     "A",
		})
		require.NoError(t, err)
	}

	planned := make([]*raft_log.CompactionJobPlan, 3)
	test.AssertIdempotent(t, func(t *testing.T) {
		tombstones.On("ListTombstones", mock.Anything).
			Return(iter.NewEmptyIterator[*metastorev1.Tombstones](), nil)

		planner := compactor.NewPlan(&raft.Log{Index: uint64(2), AppendedAt: now})
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

	test.AssertIdempotent(t, func(t *testing.T) {
		newJobs := make([]*raft_log.NewCompactionJob, 3)
		for i := range planned {
			newJobs[i] = &raft_log.NewCompactionJob{Plan: planned[i]}
		}

		update := &raft_log.CompactionPlanUpdate{NewJobs: newJobs}
		require.NoError(t, compactor.UpdatePlan(nil, update))

		planner := compactor.NewPlan(&raft.Log{Index: uint64(3), AppendedAt: now})
		job, err := planner.CreateJob()
		require.NoError(t, err)
		require.Nil(t, job)
	})

	queueStore.AssertExpectations(t)
	tombstones.AssertExpectations(t)
}

func TestCompactor_Restore(t *testing.T) {
	queueStore := new(mockcompactor.MockBlockQueueStore)
	queueStore.On("ListEntries", mock.Anything).Return(iter.NewSliceIterator([]compaction.BlockEntry{
		{Index: 0, ID: "0", Tenant: "A"},
		{Index: 1, ID: "1", Tenant: "A"},
		{Index: 2, ID: "2", Tenant: "A"},
		{Index: 3, ID: "3", Tenant: "A"},
	}))

	tombstones := new(mockcompactor.MockTombstones)
	tombstones.On("ListTombstones", mock.Anything).
		Return(iter.NewEmptyIterator[*metastorev1.Tombstones](), nil)

	compactor := NewCompactor(testConfig, queueStore, tombstones, nil)
	require.NoError(t, compactor.Restore(nil))

	planner := compactor.NewPlan(new(raft.Log))
	planned, err := planner.CreateJob()
	require.NoError(t, err)
	require.NotEmpty(t, planned)

	queueStore.AssertExpectations(t)
	tombstones.AssertExpectations(t)
}
