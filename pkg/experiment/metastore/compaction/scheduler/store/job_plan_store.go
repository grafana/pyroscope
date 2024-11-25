package store

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/store"
)

var jobPlanBucketName = []byte("compaction_job_plan")

var ErrInvalidJobPlan = errors.New("invalid job plan entry")

type JobPlanStore struct{ bucketName []byte }

func NewJobPlanStore() *JobPlanStore {
	return &JobPlanStore{bucketName: jobPlanBucketName}
}

func (s JobPlanStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(s.bucketName)
	return err
}

func (s JobPlanStore) StoreJobPlan(tx *bbolt.Tx, plan *raft_log.CompactionJobPlan) error {
	v, _ := plan.MarshalVT()
	return tx.Bucket(s.bucketName).Put([]byte(plan.Name), v)
}

func (s JobPlanStore) GetJobPlan(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error) {
	b := tx.Bucket(s.bucketName).Get([]byte(name))
	if b == nil {
		return nil, fmt.Errorf("loading job plan %s: %w", name, store.ErrorNotFound)
	}
	var v raft_log.CompactionJobPlan
	if err := v.UnmarshalVT(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJobPlan, err)
	}
	return &v, nil
}

func (s JobPlanStore) DeleteJobPlan(tx *bbolt.Tx, name string) error {
	return tx.Bucket(s.bucketName).Delete([]byte(name))
}
