package store

import (
	"go.etcd.io/bbolt"
)

type JobStore struct {
	*JobStateStore
	*JobPlanStore
}

func NewJobStore() *JobStore {
	return &JobStore{
		JobStateStore: NewJobStateStore(),
		JobPlanStore:  NewJobPlanStore(),
	}
}

func (s JobStore) CreateBuckets(tx *bbolt.Tx) error {
	if err := s.JobStateStore.CreateBuckets(tx); err != nil {
		return err
	}
	if err := s.JobPlanStore.CreateBuckets(tx); err != nil {
		return err
	}
	return nil
}
