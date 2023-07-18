package labels

import (
	"strings"

	"github.com/dgraph-io/badger/v2"
)

type Labels struct {
	db *badger.DB
}

func New(db *badger.DB) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func (ll *Labels) PutLabels(labels map[string]string) error {
	return ll.db.Update(func(txn *badger.Txn) error {
		for k, v := range labels {
			if err := txn.SetEntry(badger.NewEntry([]byte("l:"+k), nil)); err != nil {
				return err
			}
			if err := txn.SetEntry(badger.NewEntry([]byte("v:"+k+":"+v), nil)); err != nil {
				return err
			}
		}
		return nil
	})
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

//revive:disable-next-line:get-return A callback is fine
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

// Delete removes key value label pair from the storage.
// If the pair can not be found, no error is returned.
func (ll *Labels) Delete(key, value string) error {
	return ll.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte("v:" + key + ":" + value))
	})
}

//revive:disable-next-line:get-return A callback is fine
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
