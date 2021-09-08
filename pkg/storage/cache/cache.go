package cache

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu"
	"github.com/valyala/bytebufferpool"
)

type Cache struct {
	db     *badger.DB
	lfu    *lfu.Cache
	prefix string

	alwaysSave    bool
	evictionsDone chan struct{}
	flushOnce     sync.Once

	metrics *Metrics

	Codec
}

type Codec interface {
	Serialize(w io.Writer, k string, v interface{}) error
	Deserialize(k string, r io.Reader) (interface{}, error)
	New() interface{}
}

type Metrics struct {
	HitCounter  prometheus.Counter
	MissCounter prometheus.Counter
	ReadCounter prometheus.Counter

	DiskWritesHistogram prometheus.Observer
	DiskReadsHistogram  prometheus.Observer
}

func New(db *badger.DB, prefix string, metrics *Metrics) *Cache {
	evictionChannel := make(chan lfu.Eviction)
	writeBackChannel := make(chan lfu.Eviction)

	l := lfu.New()

	// eviction channel for saving cache items to disk
	l.EvictionChannel = evictionChannel
	l.WriteBackChannel = writeBackChannel
	l.TTL = 120

	cache := &Cache{
		db:  db,
		lfu: l,

		prefix:        prefix,
		evictionsDone: make(chan struct{}),

		metrics: metrics,
	}

	// start a goroutine for saving the evicted cache items to disk

	go func() {
		for {
			e, ok := <-evictionChannel
			if !ok {
				break
			}
			cache.saveToDisk(e.Key, e.Value)
		}
		cache.evictionsDone <- struct{}{}
	}()

	// start a goroutine for saving the evicted cache items to disk
	go func() {
		for {
			e, ok := <-writeBackChannel
			if !ok {
				break
			}
			cache.saveToDisk(e.Key, e.Value)
		}
	}()

	return cache
}

func (cache *Cache) Put(key string, val interface{}) {
	cache.lfu.Set(key, val)
}

func (cache *Cache) saveToDisk(key string, val interface{}) error {
	b := bytebufferpool.Get()
	defer bytebufferpool.Put(b)
	if err := cache.Serialize(b, key, val); err != nil {
		return fmt.Errorf("serialization: %w", err)
	}
	cache.metrics.DiskWritesHistogram.Observe(float64(b.Len()))
	return cache.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(cache.prefix+key), b.Bytes())
	})
}

func (cache *Cache) Flush() {
	cache.flushOnce.Do(func() {
		// evict all the items in cache
		cache.lfu.Evict(cache.lfu.Len())

		close(cache.lfu.EvictionChannel)
		close(cache.lfu.WriteBackChannel)

		// wait until all evictions are done
		<-cache.evictionsDone
	})
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (cache *Cache) Evict(percent float64) {
	cache.lfu.Evict(int(float64(cache.lfu.Len()) * percent))
}

func (cache *Cache) WriteBack() {
	cache.lfu.WriteBack(cache.lfu.Len())
}

func (cache *Cache) Delete(key string) error {
	cache.lfu.Delete(key)
	return cache.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(cache.prefix + key))
	})
}

func (cache *Cache) GetOrCreate(key string) (interface{}, error) {
	v, err := cache.get(key) // find the key from cache first
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	v = cache.New()
	cache.lfu.Set(key, v)
	return v, nil
}

func (cache *Cache) Walk(fn func(k string, v interface{})) {
	cache.lfu.Walk(fn)
}

func (cache *Cache) Lookup(key string) (interface{}, bool) {
	v, err := cache.get(key)
	if v == nil || err != nil {
		return nil, false
	}
	return v, true
}

func (cache *Cache) get(key string) (interface{}, error) {
	cache.metrics.ReadCounter.Add(1)
	return cache.lfu.GetOrSet(key, func() (interface{}, error) {
		cache.metrics.MissCounter.Add(1)
		var buf []byte
		err := cache.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(cache.prefix + key))
			if err != nil {
				return err
			}
			buf, err = item.ValueCopy(buf)
			return err
		})

		switch {
		case err == nil:
		case errors.Is(err, badger.ErrKeyNotFound):
			return nil, nil
		default:
			return nil, err
		}

		cache.metrics.DiskReadsHistogram.Observe(float64(len(buf)))
		return cache.Deserialize(key, bytes.NewReader(buf))
	})
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}
