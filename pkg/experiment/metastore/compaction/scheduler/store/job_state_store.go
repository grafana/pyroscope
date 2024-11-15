package store

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

var jobStateBucketName = []byte("compaction_job_state")

var ErrInvalidJobState = errors.New("invalid job state entry")

type JobStateStore struct{ bucketName []byte }

func NewJobStateStore() *JobStateStore {
	return &JobStateStore{bucketName: jobStateBucketName}
}

func (s JobStateStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(s.bucketName)
	return err
}

func (s JobStateStore) GetJobState(tx *bbolt.Tx, name string) (*raft_log.CompactionJobState, error) {
	b := tx.Bucket(s.bucketName).Get([]byte(name))
	if b == nil {
		return nil, fmt.Errorf("loading job state %s: %w", name, store.ErrorNotFound)
	}
	var v raft_log.CompactionJobState
	if err := v.UnmarshalVT(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJobState, err)
	}
	return &v, nil
}

func (s JobStateStore) StoreJobState(tx *bbolt.Tx, state *raft_log.CompactionJobState) error {
	v, _ := state.MarshalVT()
	return tx.Bucket(s.bucketName).Put([]byte(state.Name), v)
}

func (s JobStateStore) DeleteJobState(tx *bbolt.Tx, name string) error {
	return tx.Bucket(s.bucketName).Delete([]byte(name))
}

func (s JobStateStore) ListEntries(tx *bbolt.Tx) iter.Iterator[*raft_log.CompactionJobState] {
	return newJobEntriesIterator(tx.Bucket(s.bucketName))
}

type jobEntriesIterator struct {
	iter *store.CursorIterator
	cur  *raft_log.CompactionJobState
	err  error
}

func newJobEntriesIterator(bucket *bbolt.Bucket) *jobEntriesIterator {
	return &jobEntriesIterator{iter: store.NewCursorIter(nil, bucket.Cursor())}
}

func (x *jobEntriesIterator) Next() bool {
	if x.err != nil || !x.iter.Next() {
		return false
	}
	e := x.iter.At()
	var s raft_log.CompactionJobState
	x.err = s.UnmarshalVT(e.Value)
	if x.err != nil {
		x.err = fmt.Errorf("%w: %v", ErrInvalidJobState, x.err)
		return false
	}
	x.cur = &s
	return true
}

func (x *jobEntriesIterator) At() *raft_log.CompactionJobState { return x.cur }

func (x *jobEntriesIterator) Close() error { return x.iter.Close() }

func (x *jobEntriesIterator) Err() error {
	if err := x.iter.Err(); err != nil {
		return err
	}
	return x.err
}
