package cache

import (
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
	Bytes func(k string, v interface{}) []byte
	// FromBytes deserializes object coming from storage. Users are required to define this one
	FromBytes func(k string, v []byte) interface{}
	// New creates a new object when there's no object in cache or storage. Optional
	New func(k string) interface{}
}

func New(db *badger.DB, bound int, prefix string) *Cache {
	eviction := make(chan lfu.Eviction, 1)

	l := lfu.New()
	// new a cache with LFU(Least Frequently Used) strategy
	l.UpperBound = bound
	// 10 percent of upper for the lower bound
	l.LowerBound = bound - bound/10
	l.EvictionChannel = eviction

	cache := &Cache{
		db:          db,
		lfu:         l,
		prefix:      prefix,
		cleanupDone: make(chan struct{}),
	}

	// start a goroutine for handling the evicted item
	go func() {
		for {
			e, ok := <-eviction
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

func (cache *Cache) saveToDisk(key string, val interface{}) {
	logrus.WithFields(logrus.Fields{
		"prefix": cache.prefix,
		"key":    key,
	}).Debug("saving to disk")
	buf := cache.Bytes(key, val)
	err := cache.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(cache.prefix+key), buf))
	})
	if err != nil {
		// TODO: handle
		logrus.Errorf("error happened in saveToDisk: %v", err)
	}
}

func (cache *Cache) Flush() {
	cache.lfu.Evict(cache.lfu.Len())
	close(cache.lfu.EvictionChannel)
	<-cache.cleanupDone
}

func (cache *Cache) Get(key string) interface{} {
	lg := logrus.WithField("key", key)
	if cache.lfu.UpperBound > 0 {
		fromLfu := cache.lfu.Get(key)
		if fromLfu != nil {
			return fromLfu
		}
	} else {
		logrus.Warn("lfu is not used, only use this during debugging")
	}
	lg.Debug("lfu miss")

	var valCopy []byte
	err := cache.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(cache.prefix + key))
		if err != nil {
			// TODO: handle
			if err == badger.ErrKeyNotFound {
				return nil
			}
			logrus.Errorf("error happened when reading from badger %v", err)
		}

		err = item.Value(func(val []byte) error {
			valCopy = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			// TODO: handle
			logrus.Errorf("error happened getting value from badger %v", err)
		}
		return nil
	})

	if err != nil {
		// TODO: handle
		logrus.Errorf("error happened in badger view %v", err)
		return nil
	}

	if valCopy == nil {
		lg.Debug("storage miss")
		if cache.New == nil {
			return nil
		}
		newStruct := cache.New(key)
		cache.lfu.Set(key, newStruct)
		return newStruct
	}

	val := cache.FromBytes(key, valCopy)

	cache.lfu.Set(key, val)
	if cache.alwaysSave {
		cache.saveToDisk(key, val)
	}

	lg.Debug("storage hit")
	return val
}

func (cache *Cache) Evit(percent int) {
	cache.lfu.Evict(cache.lfu.Len() / percent)
}
