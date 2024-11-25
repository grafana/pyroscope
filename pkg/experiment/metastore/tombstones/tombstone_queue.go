package tombstones

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/tombstones/store"
)

type tombstoneQueue struct{ head, tail *tombstones }

// The only requirement to tombstoneKey is that it must be
// unique and must be received from the raft log.
type tombstoneKey string

func (k *tombstoneKey) set(t *metastorev1.Tombstones) bool {
	if t.Blocks != nil {
		*k = tombstoneKey(t.Blocks.Name)
	}
	return len(*k) > 0
}

type tombstones struct {
	store.TombstoneEntry
	next, prev *tombstones
}

func newTombstoneQueue() *tombstoneQueue { return &tombstoneQueue{} }

func (q *tombstoneQueue) push(e *tombstones) bool {
	if q.tail != nil {
		q.tail.next = e
		e.prev = q.tail
	} else {
		q.head = e
	}
	q.tail = e
	return true
}

func (q *tombstoneQueue) delete(e *tombstones) *tombstones {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		// This is the head.
		q.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		// This is the tail.
		q.tail = e.next
	}
	e.next = nil
	e.prev = nil
	return e
}

type tombstoneIter struct {
	head   *tombstones
	before int64
}

func (t *tombstoneIter) Next() bool {
	if t.head == nil {
		return false
	}
	if t.head = t.head.next; t.head == nil {
		return false
	}
	return t.head.AppendedAt < t.before
}

func (t *tombstoneIter) At() *metastorev1.Tombstones { return t.head.Tombstones }

func (t *tombstoneIter) Err() error   { return nil }
func (t *tombstoneIter) Close() error { return nil }
