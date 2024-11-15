package scheduler

import (
	"testing"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockscheduler"
)

func TestScheduler_UpdateSchedule(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	store.On("StoreJobPlan", mock.Anything, &raft_log.CompactionJobPlan{Name: "1"}).Return(nil).Once()
	store.On("StoreJobState", mock.Anything, &raft_log.CompactionJobState{Name: "1"}).Return(nil).Once()
	store.On("StoreJobState", mock.Anything, &raft_log.CompactionJobState{Name: "2"}).Return(nil).Once()
	store.On("DeleteJobPlan", mock.Anything, "3").Return(nil).Once()
	store.On("DeleteJobState", mock.Anything, "3").Return(nil).Once()

	scheduler := NewScheduler(Config{}, store)
	scheduler.queue.put(&raft_log.CompactionJobState{Name: "1", Token: 1})
	scheduler.queue.put(&raft_log.CompactionJobState{Name: "2", Token: 1})
	scheduler.queue.put(&raft_log.CompactionJobState{Name: "3", Token: 1})

	err := scheduler.UpdateSchedule(nil, &raft.Log{Index: 2}, &raft_log.CompactionPlanUpdate{
		NewJobs: []*raft_log.CompactionJobUpdate{{
			State: &raft_log.CompactionJobState{Name: "1"},
			Plan:  &raft_log.CompactionJobPlan{Name: "1"},
		}},
		AssignedJobs: []*raft_log.CompactionJobUpdate{{
			State: &raft_log.CompactionJobState{Name: "2"},
			Plan:  &raft_log.CompactionJobPlan{Name: "2"},
		}},
		CompletedJobs: []*raft_log.CompactionJobUpdate{{
			State: &raft_log.CompactionJobState{Name: "3"},
			Plan:  &raft_log.CompactionJobPlan{Name: "3"},
		}},
	})

	require.NoError(t, err)
	s := scheduler.NewSchedule(nil, &raft.Log{Index: 3})

	store.On("GetJobPlan", mock.Anything, "1").Return(new(raft_log.CompactionJobPlan), nil).Once()
	assigment, err := s.AssignJob()
	require.NoError(t, err)
	assert.NotNil(t, assigment)

	store.On("GetJobPlan", mock.Anything, "2").Return(new(raft_log.CompactionJobPlan), nil).Once()
	assigment, err = s.AssignJob()
	require.NoError(t, err)
	assert.NotNil(t, assigment)

	assigment, err = s.AssignJob()
	require.NoError(t, err)
	assert.Nil(t, assigment)

	store.AssertExpectations(t)
}

func TestScheduler_Restore(t *testing.T) {
	store := new(mockscheduler.MockJobStore)
	scheduler := NewScheduler(Config{}, store)

	store.On("ListEntries", mock.Anything).Return(iter.NewSliceIterator([]*raft_log.CompactionJobState{
		{Name: "1", Token: 1},
		{Name: "2", Token: 1},
	}))

	require.NoError(t, scheduler.Restore(nil))
	s := scheduler.NewSchedule(nil, &raft.Log{Index: 3})

	store.On("GetJobPlan", mock.Anything, "1").Return(new(raft_log.CompactionJobPlan), nil).Once()
	assigment, err := s.AssignJob()
	require.NoError(t, err)
	assert.NotNil(t, assigment)

	store.On("GetJobPlan", mock.Anything, "2").Return(new(raft_log.CompactionJobPlan), nil).Once()
	assigment, err = s.AssignJob()
	require.NoError(t, err)
	assert.NotNil(t, assigment)

	assigment, err = s.AssignJob()
	require.NoError(t, err)
	assert.Nil(t, assigment)

	store.AssertExpectations(t)
}
