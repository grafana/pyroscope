package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
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

	profilesFormatV1 byte = 1
)

// profileKey creates a key in the v:{app_name}:{profile_id} format
func profileKey(appName, profileID string) []byte {
	return profileDataPrefix.key(appName + ":" + profileID)
}

// profileTimestampKey creates a key in the t:{timestamp}:{app_name}:{profile_id} format
func profileTimestampKey(t time.Time, appName, profileID string) []byte {
	return profileTimestampPrefix.key(strconv.FormatInt(t.UnixNano(), 10) + ":" + appName + ":" + profileID)
}

// parseProfileTimestamp returns timestamp and the profile
// data key (in v:{app_name}:{profile_id} format), if the given timestamp key is valid.
func parseProfileTimestamp(k []byte) (int64, []byte, bool) {
	v, ok := profileTimestampPrefix.trim(k)
	if !ok {
		return 0, nil, false
	}
	i := bytes.IndexByte(v, ':')
	if i < 0 {
		return 0, nil, false
	}
	t, err := strconv.ParseInt(string(v[:i]), 10, 64)
	if err != nil {
		return 0, nil, false
	}
	return t, append(profileDataPrefix.bytes(), v[i+1:]...), true
}

func (s profiles) insert(appName, profileID string, v *tree.Tree, at time.Time) error {
	d, err := s.dicts.GetOrCreate(appName)
	if err != nil {
		return err
	}
	dx := d.(*dict.Dict)
	b := bufferPool.Get()
	defer func() {
		bufferPool.Put(b)
		s.dicts.Put(appName, d)
	}()

	return s.db.Update(func(txn *badger.Txn) error {
		if err = txn.Set(profileTimestampKey(at, appName, profileID), nil); err != nil {
			return err
		}

		k := profileKey(appName, profileID)
		var item *badger.Item
		item, err = txn.Get(k)
		switch {
		default:
			return err
		case errors.Is(err, badger.ErrKeyNotFound):
		case err == nil:
			err = item.Value(valueReader(dx, func(t *tree.Tree) error {
				t.Merge(v)
				v = t
				return nil
			}))
			if err != nil {
				return err
			}
		}

		if err = b.WriteByte(profilesFormatV1); err != nil {
			return err
		}
		if err = v.SerializeTruncate(dx, s.config.maxNodesSerialization, b); err != nil {
			return err
		}
		return txn.Set(k, b.Bytes())
	})
}

func (s profiles) fetch(ctx context.Context, appName string, profileIDs []string, fn func(*tree.Tree) error) error {
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
			item, err := txn.Get(profileKey(appName, profileID))
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

func (s profiles) truncateBefore(ctx context.Context, before time.Time) (err error) {
	t := before.UnixNano()
	batch := s.db.NewWriteBatch()
	defer func() {
		err = batch.Flush()
	}()
	var c int64
	return s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: profileTimestampPrefix.bytes(),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if err = ctx.Err(); err != nil {
				return err
			}
			item := it.Item()
			kt, pk, ok := parseProfileTimestamp(item.Key())
			if !ok {
				continue
			}
			if kt > t {
				return nil
			}
			if c+2 >= s.db.MaxBatchCount() {
				if err = batch.Flush(); err != nil {
					return err
				}
				batch = s.db.NewWriteBatch()
				c = 0
			}
			if err = batch.Delete(pk); err != nil {
				return err
			}
			if err = batch.Delete(item.KeyCopy(nil)); err != nil {
				return err
			}
			c += 2
		}
		return nil
	})
}
