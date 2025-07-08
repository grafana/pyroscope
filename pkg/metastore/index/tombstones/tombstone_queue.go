package tombstones

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/index/tombstones/store"
)

type tombstoneQueue struct{ head, tail *tombstones }

// The only requirement to tombstoneKey is that it must be
// unique and must be received from the raft log.
type tombstoneKey string

func (k *tombstoneKey) set(t *metastorev1.Tombstones) bool {
	switch {
	case t.Blocks != nil:
		*k = tombstoneKey(t.Blocks.Name)
	case t.Shard != nil:
		*k = tombstoneKey(t.Shard.Name)
	}
	return len(*k) > 0
}

type tombstones struct {
	store.TombstoneEntry
	next, prev *tombstones
}

func newTombstoneQueue() *tombstoneQueue { return &tombstoneQueue{} }

func (q *tombstoneQueue) push(e *tombstones) {
	if q.tail != nil {
		q.tail.next = e
		e.prev = q.tail
	} else if q.head == nil {
		q.head = e
	} else {
		panic("bug: queue has head but tail is nil")
	}
	q.tail = e
}

func (q *tombstoneQueue) delete(e *tombstones) *tombstones {
	if e.prev != nil {
		e.prev.next = e.next
	} else if e == q.head {
		// This is the head.
		q.head = e.next
	} else {
		panic("bug: attempting to delete a tombstone that is not in the queue")
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else if e == q.tail {
		// This is the tail.
		q.tail = e.prev
	} else {
		panic("bug: attempting to delete a tombstone that is not in the queue")
	}
	e.next = nil
	e.prev = nil
	return e
}

type tombstoneIter struct {
	head    *tombstones
	current *tombstones
	before  int64
}

func (t *tombstoneIter) Next() bool {
	if t.head == nil {
		return false
	}
	if t.current == nil {
		t.current = t.head
	} else {
		t.current = t.current.next
	}
	return t.current != nil && t.current.AppendedAt < t.before
}

func (t *tombstoneIter) At() *metastorev1.Tombstones { return t.current.Tombstones }

func (t *tombstoneIter) Err() error   { return nil }
func (t *tombstoneIter) Close() error { return nil }
