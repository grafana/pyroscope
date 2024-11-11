package store

import (
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/iter"
)

type JobStore struct{ bucketName []byte }

func NewJobStore(bucket []byte) *JobStore { return &JobStore{bucketName: bucket} }

func (s JobStore) bucket(tx *bbolt.Tx) *bbolt.Bucket { return tx.Bucket(s.bucketName) }

func (s JobStore) StoreJobPlan(tx *bbolt.Tx, plan *raft_log.CompactionJobPlan) error {
	v, err := plan.MarshalVT()
	if err != nil {
		return err
	}
	return s.bucket(tx).Put(JobPlanKeyPrefix.Key(plan.Name), v)
}

func (s JobStore) GetJobPlan(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error) {
	b := s.bucket(tx).Get(JobPlanKeyPrefix.Key(name))
	if b == nil {
		return nil, ErrorNotFound
	}
	var v raft_log.CompactionJobPlan
	if err := v.UnmarshalVT(b); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s JobStore) DeleteJobPlan(tx *bbolt.Tx, name string) error {
	return s.bucket(tx).Delete(JobPlanKeyPrefix.Key(name))
}

func (s JobStore) GetJobState(tx *bbolt.Tx, name string) (*raft_log.CompactionJobState, error) {
	b := s.bucket(tx).Get(JobStateKeyPrefix.Key(name))
	if b == nil {
		return nil, ErrorNotFound
	}
	var v raft_log.CompactionJobState
	if err := v.UnmarshalVT(b); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s JobStore) UpdateJobState(tx *bbolt.Tx, state *raft_log.CompactionJobState) error {
	v, err := state.MarshalVT()
	if err != nil {
		return err
	}
	return s.bucket(tx).Put(JobStateKeyPrefix.Key(state.Name), v)
}

func (s JobStore) DeleteJobState(tx *bbolt.Tx, name string) error {
	return s.bucket(tx).Delete(JobStateKeyPrefix.Key(name))
}

func (s JobStore) ListEntries(tx *bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState] {
	return newJobEntriesIterator(s.bucket(tx))
}

type jobEntriesIterator struct {
	iter *cursorIterator
	cur  *raft_log.CompactionJobState
	err  error
}

func newJobEntriesIterator(bucket *bbolt.Bucket) *jobEntriesIterator {
	return &jobEntriesIterator{iter: newCursorIter(JobStateKeyPrefix, bucket.Cursor())}
}

func (j *jobEntriesIterator) Next() bool {
	if j.err != nil || !j.iter.Next() {
		return false
	}
	var s raft_log.CompactionJobState
	j.err = s.UnmarshalVT(j.iter.At())
	if j.err != nil {
		return false
	}
	j.cur = &s
	return true
}

func (j *jobEntriesIterator) At() *raft_log.CompactionJobState { return j.cur }
func (j *jobEntriesIterator) Err() error                       { return j.err }
func (j *jobEntriesIterator) Close() error                     { return j.iter.Close() }
