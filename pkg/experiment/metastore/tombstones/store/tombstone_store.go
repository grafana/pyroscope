package store

import (
	"encoding/binary"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

var ErrInvalidTombstoneEntry = errors.New("invalid tombstone entry")

var tombstoneBucketName = []byte("tombstones")

type TombstoneEntry struct {
	Index      uint64
	AppendedAt int64
	*metastorev1.Tombstones
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
	return &tombstoneEntriesIterator{iter: store.NewCursorIter(nil, bucket.Cursor())}
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
	b := make([]byte, e.Tombstones.SizeVT())
	_, _ = e.Tombstones.MarshalToSizedBufferVT(b)
	return store.KV{Key: k, Value: b}
}

func marshalTombstoneEntryKey(e TombstoneEntry) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b[0:8], e.Index)
	binary.BigEndian.PutUint64(b[8:16], uint64(e.AppendedAt))
	return b
}

func unmarshalTombstoneEntry(dst *TombstoneEntry, e store.KV) error {
	if len(e.Key) < 16 {
		return ErrInvalidTombstoneEntry
	}
	dst.Index = binary.BigEndian.Uint64(e.Key[0:8])
	dst.AppendedAt = int64(binary.BigEndian.Uint64(e.Key[8:16]))
	dst.Tombstones = new(metastorev1.Tombstones)
	if err := dst.Tombstones.UnmarshalVT(e.Value); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTombstoneEntry, err)
	}
	return nil
}
