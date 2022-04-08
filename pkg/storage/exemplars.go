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

type exemplars struct {
	db      *db
	dicts   *db
	config  *Config
	metrics *metrics
}

var exemplarsBufferPool = bytebufferpool.Pool{}

const (
	exemplarDataPrefix      prefix = "v:"
	exemplarTimestampPrefix prefix = "t:"

	exemplarsFormatV1 byte = 1
)

// exemplarKey creates a key in the v:{app_name}:{profile_id} format
func exemplarKey(appName, profileID string) []byte {
	return exemplarDataPrefix.key(appName + ":" + profileID)
}

// exemplarTimestampKey creates a key in the t:{timestamp}:{app_name}:{profile_id} format
func exemplarTimestampKey(t time.Time, appName, profileID string) []byte {
	return exemplarTimestampPrefix.key(strconv.FormatInt(t.UnixNano(), 10) + ":" + appName + ":" + profileID)
}

// parseExemplarTimestamp returns timestamp and the profile
// data key (in v:{app_name}:{profile_id} format), if the given timestamp key is valid.
func parseExemplarTimestamp(k []byte) (int64, []byte, bool) {
	v, ok := exemplarTimestampPrefix.trim(k)
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
	return t, append(exemplarDataPrefix.bytes(), v[i+1:]...), true
}

func (e exemplars) insert(appName, profileID string, v *tree.Tree, at time.Time) error {
	d, err := e.dicts.GetOrCreate(appName)
	if err != nil {
		return err
	}
	dx := d.(*dict.Dict)
	b := exemplarsBufferPool.Get()
	defer func() {
		exemplarsBufferPool.Put(b)
		e.dicts.Put(appName, d)
	}()

	err = e.db.Update(func(txn *badger.Txn) error {
		if err = txn.Set(exemplarTimestampKey(at, appName, profileID), nil); err != nil {
			return err
		}

		k := exemplarKey(appName, profileID)
		var item *badger.Item
		item, err = txn.Get(k)
		switch {
		default:
			return err
		case errors.Is(err, badger.ErrKeyNotFound):
		case err == nil:
			err = item.Value(e.valueReader(dx, func(t *tree.Tree) error {
				t.Merge(v)
				v = t
				return nil
			}))
			if err != nil {
				return err
			}
		}

		if err = b.WriteByte(exemplarsFormatV1); err != nil {
			return err
		}
		if err = v.SerializeTruncate(dx, e.config.maxNodesSerialization, b); err != nil {
			return err
		}

		return txn.Set(k, b.Bytes())
	})

	if err != nil {
		return err
	}

	e.metrics.exemplarsWriteBytes.Observe(float64(len(b.Bytes())))
	return nil
}

func (e exemplars) fetch(ctx context.Context, appName string, profileIDs []string, fn func(*tree.Tree) error) error {
	d, ok := e.dicts.Lookup(appName)
	if !ok {
		return nil
	}
	r := e.valueReader(d.(*dict.Dict), fn)
	return e.db.View(func(txn *badger.Txn) error {
		for _, profileID := range profileIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := txn.Get(exemplarKey(appName, profileID))
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

func (e exemplars) valueReader(d *dict.Dict, fn func(*tree.Tree) error) func(val []byte) error {
	return func(val []byte) error {
		e.metrics.exemplarsReadBytes.Observe(float64(len(val)))
		r := bytes.NewReader(val)
		v, err := r.ReadByte()
		if err != nil {
			return err
		}
		switch v {
		default:
			return fmt.Errorf("unknown exemplar format version %d", v)
		case exemplarsFormatV1:
			var t *tree.Tree
			if t, err = tree.Deserialize(d, r); err != nil {
				return err
			}
			return fn(t)
		}
	}
}

func (e exemplars) truncateBefore(ctx context.Context, before time.Time) (err error) {
	for more := true; more; {
		if err = ctx.Err(); err != nil {
			return err
		}
		if more, err = e.truncateN(before, defaultBatchSize); err != nil {
			return err
		}
	}
	return nil
}

func (e exemplars) truncateN(before time.Time, count int) (bool, error) {
	beforeTs := before.UnixNano()
	var removed int
	return removed > 0, e.db.Update(func(txn *badger.Txn) (err error) {
		batch := e.db.NewWriteBatch()
		defer func() {
			err = batch.Flush()
		}()

		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: exemplarTimestampPrefix.bytes(),
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			if removed+2 > count {
				break
			}
			item := it.Item()
			keyTs, exKey, ok := parseExemplarTimestamp(item.Key())
			if !ok {
				continue
			}
			if keyTs > beforeTs {
				break
			}
			if err = batch.Delete(exKey); err != nil {
				return err
			}
			if err = batch.Delete(item.KeyCopy(nil)); err != nil {
				return err
			}
			removed += 2
		}

		e.metrics.exemplarsRemovedTotal.Add(float64(removed))
		return nil
	})
}
