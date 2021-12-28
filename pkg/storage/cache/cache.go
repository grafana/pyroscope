package cache

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
)

type Cache struct {
	db      *badger.DB
	metrics *Metrics
	codec   Codec

	prefix string
	ttl    int64

	buckets map[uint64]*bucket
}

type bucket struct {
	lock   sync.Mutex
	values map[string]*entry
	freqs  *list.List
	len    int
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
	// TODO(kolesnikovae): Measure latencies.
}

type entry struct {
	key            string
	value          interface{}
	freqNode       *list.Element
	persisted      bool
	lastAccessTime int64
}

type listEntry struct {
	entries map[*entry]struct{}
	freq    int
}

const cacheBuckets = 16 // TODO(kolesnikovae): Heuristics?

func New(c Config) *Cache {
	v := &Cache{
		db:      c.DB,
		codec:   c.Codec,
		metrics: c.Metrics,
		prefix:  c.Prefix,
		ttl:     int64(c.TTL.Seconds()),
		buckets: make(map[uint64]*bucket, cacheBuckets),
	}
	for i := uint64(0); i < cacheBuckets; i++ {
		v.buckets[i] = &bucket{
			values: make(map[string]*entry),
			freqs:  list.New(),
		}
	}
	return v
}

func (c *Cache) bucket(k []byte) *bucket {
	return c.buckets[xxhash.Sum64(k)%cacheBuckets]
}

func (b *bucket) increment(e *entry) {
	e.lastAccessTime = time.Now().Unix()
	currentPlace := e.freqNode
	var nextFreq int
	var nextPlace *list.Element
	if currentPlace == nil {
		// new entry
		nextFreq = 1
		nextPlace = b.freqs.Front()
	} else {
		// move up
		nextFreq = currentPlace.Value.(*listEntry).freq + 1
		nextPlace = currentPlace.Next()
	}

	if nextPlace == nil || nextPlace.Value.(*listEntry).freq != nextFreq {
		// create a new list entry
		li := new(listEntry)
		li.freq = nextFreq
		li.entries = make(map[*entry]struct{})
		if currentPlace != nil {
			nextPlace = b.freqs.InsertAfter(li, currentPlace)
		} else {
			nextPlace = b.freqs.PushFront(li)
		}
	}
	e.freqNode = nextPlace
	nextPlace.Value.(*listEntry).entries[e] = struct{}{}
	if currentPlace != nil {
		// remove from current position
		b.remEntry(currentPlace, e)
	}
}

func (b *bucket) remEntry(place *list.Element, e *entry) {
	entries := place.Value.(*listEntry).entries
	delete(entries, e)
	if len(entries) == 0 {
		b.freqs.Remove(place)
	}
}

// Size reports approximate number of entries in the cache.
func (c *Cache) Size() uint64 {
	var v int
	for _, b := range c.buckets {
		b.lock.Lock()
		v += b.len
		b.lock.Unlock()
	}
	return uint64(v)
}

func (c *Cache) Put(key string, val interface{}) {
	b := c.bucket([]byte(key))
	b.lock.Lock()
	b.set(key, val)
	b.lock.Unlock()
}

func (b *bucket) set(key string, value interface{}) {
	e, ok := b.values[key]
	if ok {
		e.value = value
		e.persisted = false
		b.increment(e)
		return
	}
	e = new(entry)
	e.key = key
	e.value = value
	b.values[key] = e
	b.increment(e)
	b.len++
}

func (c *Cache) Lookup(key string) (interface{}, bool) {
	b := c.bucket([]byte(key))
	b.lock.Lock()
	defer b.lock.Unlock()
	v, err := c.get(b, key)
	if v == nil || err != nil {
		return nil, false
	}
	return v, true
}

func (c *Cache) GetOrCreate(key string) (interface{}, error) {
	b := c.bucket([]byte(key))
	b.lock.Lock()
	defer b.lock.Unlock()
	v, err := c.get(b, key)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	v = c.codec.New(key)
	b.set(key, v)
	return v, nil
}

func (c *Cache) get(b *bucket, key string) (interface{}, error) {
	c.metrics.ReadsCounter.Inc()
	e, ok := b.values[key]
	if ok {
		b.increment(e)
		return e.value, nil
	}
	// Value doesn't exist in cache, load from DB.
	c.metrics.MissesCounter.Inc()
	var buf []byte // TODO(kolesnikovae): Use buffer pool?
	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(c.prefix + key))
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
	c.metrics.DBReads.Observe(float64(len(buf)))
	v, err := c.codec.Deserialize(bytes.NewReader(buf), key)
	if err != nil || v == nil {
		return nil, err
	}
	// Add entry to cache.
	e = new(entry)
	e.key = key
	e.value = v
	b.values[key] = e
	b.increment(e)
	b.len++
	return v, nil
}

func (c *Cache) Delete(key string) error {
	b := c.bucket([]byte(key))
	b.lock.Lock()
	defer b.lock.Unlock()
	if e, ok := b.values[key]; ok {
		b.deleteEntry(e)
	}
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(c.prefix + key))
	})
}

func (c *Cache) Discard(key string) {
	b := c.bucket([]byte(key))
	b.lock.Lock()
	if e, ok := b.values[key]; ok {
		b.deleteEntry(e)
	}
	b.lock.Unlock()
}

func (c *Cache) DeletePrefix(prefix string) error {
	// TODO: Lock the whole cache?
	c.DiscardPrefix(prefix)
	return c.db.DropPrefix([]byte(c.prefix + prefix))
}

func (c *Cache) DiscardPrefix(prefix string) {
	for _, b := range c.buckets {
		b.lock.Lock()
		for k, e := range b.values {
			if strings.HasPrefix(k, prefix) {
				b.deleteEntry(e)
			}
		}
		b.lock.Unlock()
	}
}

func (b *bucket) deleteEntry(e *entry) {
	delete(b.values, e.key)
	b.remEntry(e.freqNode, e)
	b.len--
}

func (c *Cache) Flush() error {
	_, err := c.Evict(1)
	return err
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (c *Cache) Evict(percent float64) (evicted int, err error) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.EvictionsDuration.Observe))
	defer timer.ObserveDuration()
	var e int
	// TODO(kolesnikovae): Run concurrently?
	for _, b := range c.buckets {
		b.lock.Lock()
		e, err = c.evictBucket(int(float64(b.len)*percent), b)
		b.lock.Unlock()
		evicted += e
		if err != nil {
			return evicted, err
		}
	}
	return evicted, err
}

func (c *Cache) evictBucket(count int, b *bucket) (evicted int, err error) {
	batch := c.newWriteBatch()
	defer func() {
		err = batch.flush()
	}()
	for i := 0; i < count; {
		if place := b.freqs.Front(); place != nil {
			for e := range place.Value.(*listEntry).entries {
				if i >= count {
					return evicted, nil
				}
				if !e.persisted {
					if err = batch.write(e); err != nil {
						return evicted, err
					}
				}
				b.deleteEntry(e)
				evicted++
				i++
			}
		}
	}
	return evicted, nil
}

func (c *Cache) WriteBack() (persisted, evicted int, err error) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.WriteBackDuration.Observe))
	defer timer.ObserveDuration()
	var evictBefore int64
	if c.ttl > 0 {
		evictBefore = time.Now().Unix() - c.ttl
	}
	var p, ev int
	// TODO(kolesnikovae): Run concurrently?
	for _, b := range c.buckets {
		b.lock.Lock()
		p, ev, err = c.writeBackBucket(evictBefore, b)
		b.lock.Unlock()
		persisted += p
		evicted += ev
		if err != nil {
			return persisted, evicted, nil
		}
	}
	return persisted, evicted, nil
}

func (c *Cache) writeBackBucket(evictBefore int64, b *bucket) (persisted, evicted int, err error) {
	batch := c.newWriteBatch()
	defer func() {
		err = batch.flush()
	}()
	for _, e := range b.values {
		if !e.persisted {
			if err = batch.write(e); err != nil {
				return persisted, evicted, err
			}
			e.persisted = true
			persisted++
		}
		if e.lastAccessTime < evictBefore {
			b.deleteEntry(e)
			evicted++
		}
	}
	return persisted, evicted, nil
}

type writeBatch struct {
	c  *Cache
	wb *badger.WriteBatch

	count int
	size  int
	err   error
}

func (c *Cache) newWriteBatch() *writeBatch {
	return &writeBatch{c: c, wb: c.db.NewWriteBatch()}
}

func (b *writeBatch) flush() error {
	if b.err != nil {
		return b.err
	}
	if b.size == 0 {
		return nil
	}
	return b.wb.Flush()
}

func (b *writeBatch) write(e *entry) error {
	var buf bytes.Buffer // TODO(kolesnikovae): Use pool with batch?
	if err := b.c.codec.Serialize(&buf, e.key, e.value); err != nil {
		b.err = fmt.Errorf("serialize: %w", err)
		return b.err
	}
	k := []byte(b.c.prefix + e.key)
	v := buf.Bytes()
	if b.size+len(v) >= int(b.c.db.MaxBatchSize()) || b.count+1 >= int(b.c.db.MaxBatchCount()) {
		if err := b.wb.Flush(); err != nil {
			b.err = err
			return err
		}
		b.wb = b.c.db.NewWriteBatch()
		b.size = 0
		b.count = 0
	}
	if err := b.wb.Set(k, v); err != nil {
		b.err = err
		return err
	}
	b.size += len(v)
	b.count++
	b.c.metrics.DBWrites.Observe(float64(len(v)))
	return nil
}
