package labels

import (
	"bytes"

	badgerdb "github.com/dgraph-io/badger/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/kv"
	"github.com/pyroscope-io/pyroscope/pkg/storage/kv/badger"
)

type Labels struct {
	db kv.Storage
}

func New(db kv.Storage) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func (ll *Labels) Put(key, val string) error {
	kk := "l:" + key
	kv := "v:" + key + ":" + val
	if err := ll.db.Set([]byte(kk), []byte{}); err != nil {
		return err
	}
	if err := ll.db.Set([]byte(kv), []byte{}); err != nil {
		return err
	}
	return nil
}

func (ll *Labels) GetKeys(cb func(k string) bool) error {
	switch s := ll.db.(type) {
	case *badger.Service:
		if err := s.View(func(txn *badgerdb.Txn) error {
			opts := badgerdb.DefaultIteratorOptions
			opts.Prefix = []byte("l:")
			opts.PrefetchValues = false

			it := txn.NewIterator(opts)
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				key := it.Item().Key()
				if !cb(string(key[2:])) {
					return nil
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	default:
	}
	return nil
}

func (ll *Labels) GetValues(k string, cb func(v string) bool) error {
	switch s := ll.db.(type) {
	case *badger.Service:
		if err := s.View(func(txn *badgerdb.Txn) error {
			opts := badgerdb.DefaultIteratorOptions
			opts.Prefix = []byte("v:" + k + ":")
			opts.PrefetchValues = false

			it := txn.NewIterator(opts)
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				key := it.Item().Key()
				li := bytes.LastIndex(key, []byte{':'}) + 1
				if !cb(string(key[li:])) {
					return nil
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	default:
	}
	return nil
}
