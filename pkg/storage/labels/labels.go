package labels

import (
	"strings"

	"github.com/dgraph-io/badger/v2"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type Labels struct {
	db *badger.DB
}

func New(_, db *badger.DB) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func (ll *Labels) Put(key, val string) {
	kk := "l:" + key
	kv := "v:" + key + ":" + val
	// ks := "h:" + key + ":" + val + ":" + stree
	err := ll.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(kk), []byte{}))
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
	err = ll.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(kv), []byte{}))
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
	// err = ll.db.Update(func(txn *badger.Txn) error {
	// 	return txn.SetEntry(badger.NewEntry([]byte(ks), []byte{}))
	// })
	// if err != nil {
	// 	// TODO: handle
	// 	panic(err)
	// }
}

func (ll *Labels) GetKeys(cb func(k string) bool) {
	err := ll.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("l:")
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			shouldContinue := cb(string(k[2:]))
			if !shouldContinue {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
}

func (ll *Labels) GetValues(key string, cb func(v string) bool) {
	err := ll.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("v:" + key + ":")
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			ks := string(k)
			li := strings.LastIndex(ks, ":") + 1
			shouldContinue := cb(ks[li:])
			if !shouldContinue {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
}
