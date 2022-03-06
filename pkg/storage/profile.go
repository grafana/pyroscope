package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type profiles struct {
	db     *db
	dicts  *db
	config *Config
}

var bufferPool = bytebufferpool.Pool{}

const (
	profileDataPrefix      prefix = "v:"
	profileTimestampPrefix prefix = "t:"

	// t:{timestamp}:{profile_id}
	profileTimestampMinLen = len(profileTimestampPrefix) + 8 + 1

	profilesFormatV1 byte = 1
)

func profileTimestamp(t time.Time, k []byte) []byte {
	b := make([]byte, profileTimestampMinLen, 32)
	b[0], b[1], b[9] = 't', ':', ':'
	binary.LittleEndian.PutUint64(b[2:], uint64(t.UnixNano()))
	return append(b, k...)
}

// parseProfileTimestamp returns timestamp and the profile
// data key, if the given timestamp key is valid.
func parseProfileTimestamp(k []byte) (int64, []byte, bool) {
	if v, ok := profileTimestampPrefix.trim(k); ok && len(k) > profileTimestampMinLen {
		return int64(binary.LittleEndian.Uint64(v[:8])), append(profileDataPrefix.bytes(), v[9:]...), true
	}
	return 0, nil, false
}

func (s profiles) Insert(appName, profileID string, v *tree.Tree, at time.Time) error {
	d, err := s.dicts.GetOrCreate(appName)
	if err != nil {
		return err
	}
	dx := d.(*dict.Dict)
	k := []byte(profileID)
	b := bufferPool.Get()
	defer func() {
		bufferPool.Put(b)
		s.dicts.Put(appName, d)
	}()

	return s.db.Update(func(txn *badger.Txn) error {
		k = profileDataPrefix.key(appName + "." + profileID)
		if err = txn.Set(profileTimestamp(at, k), nil); err != nil {
			return err
		}

		var item *badger.Item
		item, err = txn.Get(k)
		switch {
		default:
			return err
		case errors.Is(err, badger.ErrKeyNotFound):
		case err == nil:
			return item.Value(valueReader(dx, func(t *tree.Tree) error {
				t.Merge(v)
				v = t
				return nil
			}))
		}

		if err = b.WriteByte(profilesFormatV1); err != nil {
			return err
		}
		if err = v.SerializeTruncate(dx, s.config.maxNodesSerialization, b); err == nil {
			return txn.Set(k, b.Bytes())
		}
		return err
	})
}

func (s profiles) Fetch(ctx context.Context, appName string, profileIDs []string, fn func(*tree.Tree) error) error {
	d, ok := s.dicts.Lookup(appName)
	if !ok {
		return nil
	}
	r := valueReader(d.(*dict.Dict), fn)
	return s.db.View(func(txn *badger.Txn) error {
		for _, profileID := range profileIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := txn.Get(profileDataPrefix.key(appName + "." + profileID))
			switch {
			default:
				return err
			case errors.Is(err, badger.ErrKeyNotFound):
			case err == nil:
				if err = item.Value(r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func valueReader(d *dict.Dict, fn func(*tree.Tree) error) func(val []byte) error {
	return func(val []byte) error {
		r := bytes.NewReader(val)
		v, err := r.ReadByte()
		if err != nil {
			return err
		}
		switch v {
		default:
			return fmt.Errorf("unknown profile format version %d", v)
		case profilesFormatV1:
			var t *tree.Tree
			if t, err = tree.Deserialize(d, r); err != nil {
				return err
			}
			return fn(t)
		}
	}
}

func (s profiles) Truncate(ctx context.Context, before time.Time) error {
	return truncateBefore(ctx, s.db.DB, before)
}

func truncateBefore(ctx context.Context, x *badger.DB, before time.Time) (err error) {
	t := before.UnixNano()
	batch := newWriteBatch(x)
	defer func() {
		err = batch.flush()
	}()
	return x.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: profileTimestampPrefix.bytes(),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if err = ctx.Err(); err != nil {
				return err
			}
			k := it.Item().Key()
			kt, pk, ok := parseProfileTimestamp(k)
			if !ok || kt > t {
				continue
			}
			if err = batch.delete(pk); err != nil {
				return err
			}
			if err = batch.delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

type writeBatch struct {
	db *badger.DB
	wb *badger.WriteBatch

	count int
	size  int
	err   error
}

func newWriteBatch(x *badger.DB) *writeBatch { return &writeBatch{wb: x.NewWriteBatch()} }

func (b *writeBatch) flush() error {
	if b.err != nil {
		return b.err
	}
	if b.size == 0 {
		return nil
	}
	return b.wb.Flush()
}

func (b *writeBatch) write(k, v []byte) error {
	if b.size+len(v) >= int(b.db.MaxBatchSize()) || b.count+1 >= int(b.db.MaxBatchCount()) {
		if err := b.wb.Flush(); err != nil {
			b.err = err
			return err
		}
		b.wb = b.db.NewWriteBatch()
		b.size = 0
		b.count = 0
	}
	if err := b.wb.Set(k, v); err != nil {
		b.err = err
		return err
	}
	b.size += len(v)
	b.count++
	return nil
}

func (b *writeBatch) delete(k []byte) error {
	if b.count+1 >= int(b.db.MaxBatchCount()) {
		if err := b.wb.Flush(); err != nil {
			b.err = err
			return err
		}
		b.wb = b.db.NewWriteBatch()
		b.size = 0
		b.count = 0
	}
	if err := b.wb.Delete(k); err != nil {
		b.err = err
		return err
	}
	b.count++
	return nil
}
