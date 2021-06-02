package cache

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgrijalva/lfu-go"
)

type Cache struct {
	db          *badger.DB
	lfu         *lfu.Cache
	prefix      string
	alwaysSave  bool
	cleanupDone chan struct{}

	// Bytes serializes objects before they go into storage. Users are required to define this one
	Bytes func(k string, v interface{}) ([]byte, error)
	// FromBytes deserializes object coming from storage. Users are required to define this one
	FromBytes func(k string, v []byte) (interface{}, error)
	// New creates a new object when there's no object in cache or storage. Optional
	New func(k string) interface{}
}

func New(db *badger.DB, bound int, prefix string) *Cache {
	l := lfu.New()
	// TODO: figure out how to set these
	l.UpperBound = bound
	l.LowerBound = bound - bound/10
	ech := make(chan lfu.Eviction, 1)
	l.EvictionChannel = ech
	cache := &Cache{
		db:          db,
		lfu:         l,
		prefix:      prefix,
		cleanupDone: make(chan struct{}),
	}
	go func() {
		for {
			e, ok := <-ech
			if !ok {
				break
			}
			cache.saveToDisk(e.Key, e.Value)
		}
		cache.cleanupDone <- struct{}{}
	}()
	return cache
}

func (cache *Cache) Put(key string, val interface{}) {
	cache.lfu.Set(key, val)
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}
}

func (cache *Cache) saveToDisk(key string, val interface{}) error {
	logrus.WithFields(logrus.Fields{
		"prefix": cache.prefix,
		"key":    key,
	}).Debug("saving to disk")

	// serialize the key and value
	buf, err := cache.Bytes(key, val)
	if err != nil {
		return fmt.Errorf("serialize key and value: %v", err)
	}

	// update the kv to badger
	if err := cache.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(cache.prefix+key), buf))
	}); err != nil {
		return fmt.Errorf("save to disk: %v", err)
	}
	return nil
}

func (cache *Cache) Flush() {
	cache.lfu.Evict(cache.lfu.Len())
	close(cache.lfu.EvictionChannel)
	<-cache.cleanupDone
}

func (cache *Cache) Get(key string) (interface{}, error) {
	// find the key from cache first
	if cache.lfu.UpperBound > 0 {
		val := cache.lfu.Get(key)
		if val != nil {
			return val, nil
		}
	} else {
		logrus.Warn("lfu is not used, only use this during debugging")
	}
	logrus.WithField("key", key).Debug("lfu miss")

	var copied []byte
	// read the value from badger
	if err := cache.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(cache.prefix + key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}

			return fmt.Errorf("read from badger: %v", err)
		}

		if err := item.Value(func(val []byte) error {
			copied = append([]byte{}, val...)
			return nil
		}); err != nil {
			return fmt.Errorf("retrieve value from item: %v", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("badger view: %v", err)
	}

	// if it's not found from badger, create a new object
	if copied == nil {
		logrus.WithField("key", key).Debug("storage miss")

		if cache.New == nil {
			return nil, errors.New("cache's New function is nil")
		}

		newVal := cache.New(key)
		cache.lfu.Set(key, newVal)
		return newVal, nil
	}

	// deserialize the object from storage
	val, err := cache.FromBytes(key, copied)
	if err != nil {
		return nil, fmt.Errorf("deserialize the object: %v", err)
	}
	cache.lfu.Set(key, val)
	// if it needs to save to disk
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}

	logrus.WithField("key", key).Debug("storage hit")
	return val, nil
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}
