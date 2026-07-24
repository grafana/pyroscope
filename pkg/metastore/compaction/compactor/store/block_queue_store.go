package store

import (
	"encoding/binary"
	"errors"

	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/iter"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/metastore/store"
)

var ErrInvalidBlockEntry = errors.New("invalid block entry")

var (
	blockQueueBucketName = []byte("compaction_block_queue")
	// Block sizes are stored separately, keyed identically to the block
	// entries; an entry without a size record loads with Size 0.
	blockQueueSizeBucketName = []byte("compaction_block_queue_size")
)

// BlockQueueStore provides methods to store and retrieve block queues.
// The store is optimized for two cases: load the entire queue (preserving
// the original order) and remove an entry from the queue.
//
// Compactor maintains an in-memory queue of blocks to compact, therefore
// the store never reads individual entries.
//
// NOTE(kolesnikovae): We can leverage the fact that removed entries are
// always ordered in ascending order by index and use the same cursor when
// removing entries from the database:
// DeleteEntry(*bbolt.Tx, ...store.BlockEntry) error
type BlockQueueStore struct {
	bucketName     []byte
	sizeBucketName []byte
}

func NewBlockQueueStore() *BlockQueueStore {
	return &BlockQueueStore{
		bucketName:     blockQueueBucketName,
		sizeBucketName: blockQueueSizeBucketName,
	}
}

func (s BlockQueueStore) CreateBuckets(tx *bbolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(s.bucketName); err != nil {
		return err
	}
	_, err := tx.CreateBucketIfNotExists(s.sizeBucketName)
	return err
}

func (s BlockQueueStore) StoreEntry(tx *bbolt.Tx, entry compaction.BlockEntry) error {
	e := marshalBlockEntry(entry)
	if err := tx.Bucket(s.bucketName).Put(e.Key, e.Value); err != nil {
		return err
	}
	return tx.Bucket(s.sizeBucketName).Put(e.Key, marshalBlockEntrySize(entry.Size))
}

func (s BlockQueueStore) DeleteEntry(tx *bbolt.Tx, index uint64, id string) error {
	k := marshalBlockEntryKey(index, id)
	if err := tx.Bucket(s.bucketName).Delete(k); err != nil {
		return err
	}
	return tx.Bucket(s.sizeBucketName).Delete(k)
}

func (s BlockQueueStore) ListEntries(tx *bbolt.Tx) iter.Iterator[compaction.BlockEntry] {
	return newBlockEntriesIterator(tx.Bucket(s.bucketName), tx.Bucket(s.sizeBucketName))
}

type blockEntriesIterator struct {
	iter  *store.CursorIterator
	sizes *bbolt.Bucket
	cur   compaction.BlockEntry
	err   error
}

func newBlockEntriesIterator(bucket, sizes *bbolt.Bucket) *blockEntriesIterator {
	return &blockEntriesIterator{iter: store.NewCursorIter(bucket.Cursor()), sizes: sizes}
}

func (x *blockEntriesIterator) Next() bool {
	if x.err != nil || !x.iter.Next() {
		return false
	}
	kv := x.iter.At()
	if x.err = unmarshalBlockEntry(&x.cur, kv); x.err != nil {
		return false
	}
	x.cur.Size, x.err = unmarshalBlockEntrySize(x.sizes, kv.Key)
	return x.err == nil
}

func (x *blockEntriesIterator) At() compaction.BlockEntry { return x.cur }

func (x *blockEntriesIterator) Close() error { return x.iter.Close() }

func (x *blockEntriesIterator) Err() error {
	if err := x.iter.Err(); err != nil {
		return err
	}
	return x.err
}

func marshalBlockEntry(e compaction.BlockEntry) store.KV {
	k := marshalBlockEntryKey(e.Index, e.ID)
	b := make([]byte, 8+4+4+len(e.Tenant))
	binary.BigEndian.PutUint64(b[0:8], uint64(e.AppendedAt))
	binary.BigEndian.PutUint32(b[8:12], e.Level)
	binary.BigEndian.PutUint32(b[12:16], e.Shard)
	copy(b[16:], e.Tenant)
	return store.KV{Key: k, Value: b}
}

func marshalBlockEntryKey(index uint64, id string) []byte {
	b := make([]byte, 8+len(id))
	binary.BigEndian.PutUint64(b, index)
	copy(b[8:], id)
	return b
}

func unmarshalBlockEntry(dst *compaction.BlockEntry, e store.KV) error {
	if len(e.Key) < 8 || len(e.Value) < 16 {
		return ErrInvalidBlockEntry
	}
	dst.Index = binary.BigEndian.Uint64(e.Key)
	dst.ID = string(e.Key[8:])
	dst.AppendedAt = int64(binary.BigEndian.Uint64(e.Value[0:8]))
	dst.Level = binary.BigEndian.Uint32(e.Value[8:12])
	dst.Shard = binary.BigEndian.Uint32(e.Value[12:16])
	dst.Tenant = string(e.Value[16:])
	return nil
}

// blockEntrySizeValueLen is the fixed length of a size bucket value: an
// 8-byte big-endian Size.
const blockEntrySizeValueLen = 8

func marshalBlockEntrySize(size uint64) []byte {
	b := make([]byte, blockEntrySizeValueLen)
	binary.BigEndian.PutUint64(b, size)
	return b
}

// unmarshalBlockEntrySize decodes the size bucket value for key;
// a missing record loads as size 0.
func unmarshalBlockEntrySize(sizes *bbolt.Bucket, key []byte) (uint64, error) {
	if sizes == nil {
		return 0, nil
	}
	v := sizes.Get(key)
	if v == nil {
		return 0, nil
	}
	if len(v) != blockEntrySizeValueLen {
		return 0, ErrInvalidBlockEntry
	}
	return binary.BigEndian.Uint64(v), nil
}
