package store

import (
	"encoding/binary"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/metastore/store"
)

var ErrInvalidTombstoneEntry = errors.New("invalid tombstone entry")

var tombstoneBucketName = []byte("tombstones")

type TombstoneEntry struct {
	// Key the entry was stored under. This is needed for backward
	// compatibility: if the logic for generating keys changes, we
	// will still be able to delete old entries.
	key        []byte
	Index      uint64
	AppendedAt int64
	*metastorev1.Tombstones
}

func (e TombstoneEntry) Name() string {
	switch {
	case e.Blocks != nil:
		return e.Blocks.Name
	case e.Shard != nil:
		return e.Shard.Name
	default:
		return ""
	}
}

type TombstoneStore struct{ bucketName []byte }

func NewTombstoneStore() *TombstoneStore {
	return &TombstoneStore{bucketName: tombstoneBucketName}
}

func (s *TombstoneStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(s.bucketName)
	return err
}

func (s *TombstoneStore) StoreTombstones(tx *bbolt.Tx, entry TombstoneEntry) error {
	kv := marshalTombstoneEntry(entry)
	return tx.Bucket(s.bucketName).Put(kv.Key, kv.Value)
}

func (s *TombstoneStore) DeleteTombstones(tx *bbolt.Tx, entry TombstoneEntry) error {
	return tx.Bucket(s.bucketName).Delete(marshalTombstoneEntryKey(entry))
}

func (s *TombstoneStore) ListEntries(tx *bbolt.Tx) iter.Iterator[TombstoneEntry] {
	return newTombstoneEntriesIterator(tx.Bucket(s.bucketName))
}

type tombstoneEntriesIterator struct {
	iter *store.CursorIterator
	cur  TombstoneEntry
	err  error
}

func newTombstoneEntriesIterator(bucket *bbolt.Bucket) *tombstoneEntriesIterator {
	return &tombstoneEntriesIterator{iter: store.NewCursorIter(bucket.Cursor())}
}

func (x *tombstoneEntriesIterator) Next() bool {
	if x.err != nil || !x.iter.Next() {
		return false
	}
	x.err = unmarshalTombstoneEntry(&x.cur, x.iter.At())
	return x.err == nil
}

func (x *tombstoneEntriesIterator) At() TombstoneEntry { return x.cur }

func (x *tombstoneEntriesIterator) Close() error { return x.iter.Close() }

func (x *tombstoneEntriesIterator) Err() error {
	if err := x.iter.Err(); err != nil {
		return err
	}
	return x.err
}

func marshalTombstoneEntry(e TombstoneEntry) store.KV {
	k := marshalTombstoneEntryKey(e)
	b := make([]byte, e.SizeVT())
	_, _ = e.MarshalToSizedBufferVT(b)
	return store.KV{Key: k, Value: b}
}

func marshalTombstoneEntryKey(e TombstoneEntry) []byte {
	if e.key != nil {
		b := make([]byte, len(e.key))
		copy(b, e.key)
		return b
	}
	name := e.Name()
	b := make([]byte, 16+len(name))
	binary.BigEndian.PutUint64(b[0:8], e.Index)
	binary.BigEndian.PutUint64(b[8:16], uint64(e.AppendedAt))
	copy(b[16:], name)
	return b
}

func unmarshalTombstoneEntry(e *TombstoneEntry, kv store.KV) error {
	if len(kv.Key) < 16 {
		return ErrInvalidTombstoneEntry
	}
	e.key = make([]byte, len(kv.Key))
	copy(e.key, kv.Key)
	e.Index = binary.BigEndian.Uint64(kv.Key[0:8])
	e.AppendedAt = int64(binary.BigEndian.Uint64(kv.Key[8:16]))
	e.Tombstones = new(metastorev1.Tombstones)
	if err := e.UnmarshalVT(kv.Value); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTombstoneEntry, err)
	}
	return nil
}
