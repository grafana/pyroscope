package cache

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/bytebufferpool"
	"golang.org/x/sync/errgroup"
)

type Cache struct {
	db      *badger.DB
	metrics *Metrics
	codec   Codec

	prefix string
	ttl    int64

	buckets []*bucket
}

type bucket struct {
	m      sync.Mutex
	values map[string]*entry
	freqs  *list.List
	len    int
}

type Config struct {
	*badger.DB
	*Metrics
	Codec

	Buckets int
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

const (
	defaultMaxCacheBuckets = 1
	defaultCacheBucketSize = 1 << 10
)

func New(c Config) *Cache {
	buckets := c.Buckets
	if buckets == 0 {
		buckets = int(math.Max(float64(runtime.NumCPU()), float64(defaultMaxCacheBuckets)))
	}
	v := &Cache{
		db:      c.DB,
		codec:   c.Codec,
		metrics: c.Metrics,
		prefix:  c.Prefix,
		ttl:     int64(c.TTL.Seconds()),
		buckets: make([]*bucket, buckets),
	}
	for i := 0; i < buckets; i++ {
		v.buckets[i] = &bucket{
			values: make(map[string]*entry, defaultCacheBucketSize),
			freqs:  list.New(),
		}
	}
	return v
}

// Size reports approximate number of entries in the cache.
func (c *Cache) Size() uint64 {
	var v int
	for _, b := range c.buckets {
		b.m.Lock()
		v += b.len
		b.m.Unlock()
	}
	return uint64(v)
}

func (c *Cache) Put(k string, v interface{}) {
	b := c.bucket(k)
	b.m.Lock()
	b.set(k, v, false)
	b.m.Unlock()
}

func (c *Cache) Lookup(key string) (interface{}, bool) {
	b := c.bucket(key)
	b.m.Lock()
	v, err := c.get(b, key)
	b.m.Unlock()
	return v, v != nil && err == nil
}

func (c *Cache) GetOrCreate(key string) (interface{}, error) {
	b := c.bucket(key)
	b.m.Lock()
	defer b.m.Unlock()
	v, err := c.get(b, key)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	v = c.codec.New(key)
	b.set(key, v, true)
	return v, nil
}

func (c *Cache) get(b *bucket, key string) (v interface{}, err error) {
	c.metrics.ReadsCounter.Inc()
	e, ok := b.values[key]
	if ok {
		b.increment(e)
		return e.value, nil
	}
	c.metrics.MissesCounter.Inc()
	err = c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(c.prefix + key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			c.metrics.DBReads.Observe(float64(len(val)))
			v, err = c.codec.Deserialize(bytes.NewReader(val), key)
			return err
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		err = nil
	}
	return v, err
}

func (c *Cache) Delete(key string) error {
	b := c.bucket(key)
	b.m.Lock()
	defer b.m.Unlock()
	if e, ok := b.values[key]; ok {
		b.deleteEntry(e)
	}
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(c.prefix + key))
	})
}

func (c *Cache) Discard(key string) {
	b := c.bucket(key)
	b.m.Lock()
	if e, ok := b.values[key]; ok {
		b.deleteEntry(e)
	}
	b.m.Unlock()
}

func (c *Cache) DeletePrefix(prefix string) error {
	c.DiscardPrefix(prefix)
	return c.db.DropPrefix([]byte(c.prefix + prefix))
}

func (c *Cache) DiscardPrefix(prefix string) {
	g, _ := errgroup.WithContext(context.Background())
	for _, b := range c.buckets {
		b := b
		g.Go(func() error {
			b.m.Lock()
			defer b.m.Unlock()
			for k, e := range b.values {
				if strings.HasPrefix(k, prefix) {
					b.deleteEntry(e)
				}
			}
			return nil
		})
	}
	_ = g.Wait()
}

func (c *Cache) bucket(k string) *bucket {
	return c.buckets[xxhash.Sum64String(k)%uint64(len(c.buckets))]
}

func (c *Cache) Flush() error { return c.writeBack(true) }

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (c *Cache) Evict(percent float64) error {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.EvictionsDuration.Observe))
	defer timer.ObserveDuration()
	g, _ := errgroup.WithContext(context.Background())
	for _, b := range c.buckets {
		b := b
		g.Go(func() error {
			return c.evictBucket(percent, b)
		})
	}
	return g.Wait()
}

func (c *Cache) evictBucket(percent float64, b *bucket) (err error) {
	w := newBatchedWriter(c)
	defer func() {
		err = w.flush()
	}()
	b.m.Lock()
	defer b.m.Unlock()
	count := int(float64(b.len) * percent)
	for i := 0; i < count; {
		if place := b.freqs.Front(); place != nil {
			for e := range place.Value.(*listEntry).entries {
				if i >= count {
					return nil
				}
				// Entry is evicted from memory even
				// if it has not been stored on disk.
				i++
				b.deleteEntry(e)
				if !e.persisted {
					if err = w.write(e); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c *Cache) WriteBack() error { return c.writeBack(false) }

func (c *Cache) writeBack(force bool) error {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.WriteBackDuration.Observe))
	defer timer.ObserveDuration()
	var evictBefore int64
	if c.ttl > 0 {
		evictBefore = time.Now().Unix() - c.ttl
	}
	g, _ := errgroup.WithContext(context.Background())
	for _, b := range c.buckets {
		b := b
		g.Go(func() error {
			return c.writeBackBucket(evictBefore, b, force)
		})
	}
	return g.Wait()
}

func (c *Cache) writeBackBucket(evictBefore int64, b *bucket, force bool) (err error) {
	w := newBatchedWriter(c)
	defer func() {
		err = w.flush()
	}()
	b.m.Lock()
	defer b.m.Unlock()
	for _, e := range b.values {
		if !e.persisted || force {
			if err = w.write(e); err != nil {
				return err
			}
			e.persisted = true
		}
		if e.lastAccessTime < evictBefore {
			b.deleteEntry(e)
		}
	}
	return nil
}

func (b *bucket) remEntry(place *list.Element, e *entry) {
	entries := place.Value.(*listEntry).entries
	delete(entries, e)
	if len(entries) == 0 {
		b.freqs.Remove(place)
	}
}

func (b *bucket) set(key string, value interface{}, persisted bool) {
	e, ok := b.values[key]
	if !ok {
		e = new(entry)
		b.len++
	}
	e.key = key
	e.value = value
	e.persisted = persisted
	b.values[key] = e
	b.increment(e)
}

func (b *bucket) deleteEntry(e *entry) {
	delete(b.values, e.key)
	b.remEntry(e.freqNode, e)
	b.len--
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

func newBatchedWriter(c *Cache) *batchedWriter {
	return &batchedWriter{c: c, wb: c.db.NewWriteBatch()}
}

type batchedWriter struct {
	c  *Cache
	wb *badger.WriteBatch

	count int
	size  int
	err   error
}

func (b *batchedWriter) flush() error {
	if b.err != nil {
		return b.err
	}
	if b.size == 0 {
		return nil
	}
	return b.wb.Flush()
}

var bufPool bytebufferpool.Pool

func (b *batchedWriter) write(e *entry) error {
	buf := bufPool.Get()
	defer bufPool.Put(buf)
	if err := b.c.codec.Serialize(buf, e.key, e.value); err != nil {
		b.err = fmt.Errorf("serialize: %w", err)
		return b.err
	}
	k := []byte(b.c.prefix + e.key)
	v := make([]byte, buf.Len())
	// Note that the copy is required because badger DB
	// references value bytes, and we use buffer pool.
	copy(v, buf.Bytes())
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
