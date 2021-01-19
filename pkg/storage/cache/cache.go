package cache

import (
	log "github.com/sirupsen/logrus"

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

func (cache *Cache) saveToDisk(key string, val interface{}) {
	log.WithFields(log.Fields{
		"prefix": cache.prefix,
		"key":    key,
	}).Info("saving to disk")
	buf := cache.Bytes(key, val)
	err := cache.db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(badger.NewEntry([]byte(cache.prefix+key), buf))
	})
	if err != nil {
		// TODO: handle
		panic(err)
	}
}

func (cache *Cache) Flush() {
	cache.lfu.Evict(cache.lfu.Len())
	close(cache.lfu.EvictionChannel)
	<-cache.cleanupDone
}

func (cache *Cache) Get(key string) interface{} {
	lg := log.WithField("key", key)
	if cache.lfu.UpperBound > 0 {
		fromLfu := cache.lfu.Get(key)
		if fromLfu != nil {
			return fromLfu
		}
	} else {
		log.Warn("lfu is not used, only use this during debugging")
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
			panic(err)
		}

		err = item.Value(func(val []byte) error {
			valCopy = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			// TODO: handle
			panic(err)
		}
		return nil
	})

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

	if err != nil {
		// TODO: handle
		panic(err)
	}

	lg.Debug("storage hit")
	return val
}
