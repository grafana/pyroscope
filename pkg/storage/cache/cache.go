package cache

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/storage/cache/lfu"
)

// TODO(kolesnikovae): Move to storage/db?

type Cache struct {
	db      *badger.DB
	lfu     *lfu.Cache
	metrics *Metrics
	codec   Codec

	prefix string
	ttl    time.Duration

	evictionsDone chan struct{}
	flushOnce     sync.Once
}

type Config struct {
	*badger.DB
	*Metrics
	Codec

	// Prefix for badger DB keys.
	Prefix string
	// TTL specifies number of seconds an item can reside in cache after
	// the last access. An obsolete item is evicted. Setting TTL to less
	// than a second disables time-based eviction.
	TTL time.Duration
}

// Codec is a shorthand of coder-decoder. A Codec implementation
// is responsible for type conversions and binary representation.
type Codec interface {
	Serialize(w io.Writer, key string, value interface{}) error
	Deserialize(r io.Reader, key string) (interface{}, error)
	// New returns a new instance of the type. The method is
	// called by GetOrCreate when an item can not be found by
	// the given key.
	New(key string) interface{}
}

type Metrics struct {
	MissesCounter     prometheus.Counter
	ReadsCounter      prometheus.Counter
	DBWrites          prometheus.Observer
	DBReads           prometheus.Observer
	WriteBackDuration prometheus.Observer
	EvictionsDuration prometheus.Observer
}

func New(c Config) *Cache {
	cache := &Cache{
		lfu:           lfu.New(),
		db:            c.DB,
		codec:         c.Codec,
		metrics:       c.Metrics,
		prefix:        c.Prefix,
		ttl:           c.TTL,
		evictionsDone: make(chan struct{}),
	}

	evictionChannel := make(chan lfu.Eviction)
	writeBackChannel := make(chan lfu.Eviction)

	// eviction channel for saving cache items to disk
	cache.lfu.EvictionChannel = evictionChannel
	cache.lfu.WriteBackChannel = writeBackChannel
	cache.lfu.TTL = int64(c.TTL.Seconds())

	// start a goroutine for saving the evicted cache items to disk
	// TODO(kolesnikovae): these errors should be at least logged.
	//  Perhaps, it will be better if we move it outside of the cache.
	//  Taking into account that writes almost always happen in batches,
	//  We should definitely take advantage of BadgerDB write batch API.

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
	if err := cache.codec.Serialize(b, key, val); err != nil {
		return fmt.Errorf("serialization: %w", err)
	}
	cache.metrics.DBWrites.Observe(float64(b.Len()))
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
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.EvictionsDuration.Observe))
	cache.lfu.Evict(int(float64(cache.lfu.Len()) * percent))
	timer.ObserveDuration()
}

func (cache *Cache) WriteBack() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(cache.metrics.WriteBackDuration.Observe))
	cache.lfu.WriteBack()
	timer.ObserveDuration()
}

func (cache *Cache) Delete(key string) error {
	cache.lfu.Delete(key)
	return cache.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(cache.prefix + key))
	})
}

func (cache *Cache) Discard(key string) {
	cache.lfu.Delete(key)
}

func (cache *Cache) DiscardPrefix(prefix string) {
	cache.lfu.DeletePrefix(prefix)
}

func (cache *Cache) GetOrCreate(key string) (interface{}, error) {
	v, err := cache.get(key) // find the key from cache first
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	v = cache.codec.New(key)
	cache.lfu.Set(key, v)
	return v, nil
}

func (cache *Cache) Lookup(key string) (interface{}, bool) {
	v, err := cache.get(key)
	if v == nil || err != nil {
		return nil, false
	}
	return v, true
}

func (cache *Cache) get(key string) (interface{}, error) {
	cache.metrics.ReadsCounter.Inc()
	return cache.lfu.GetOrSet(key, func() (interface{}, error) {
		cache.metrics.MissesCounter.Inc()
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

		cache.metrics.DBReads.Observe(float64(len(buf)))
		return cache.codec.Deserialize(bytes.NewReader(buf), key)
	})
}

func (cache *Cache) Size() uint64 {
	return uint64(cache.lfu.Len())
}
