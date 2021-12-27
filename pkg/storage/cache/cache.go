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

	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
)

type Cache struct {
	db      *badger.DB
	metrics *Metrics
	codec   Codec

	prefix string
	ttl    int64

	lock   sync.Mutex
	values map[string]*cacheEntry
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

type cacheEntry struct {
	key            string
	value          interface{}
	freqNode       *list.Element
	persisted      bool
	lastAccessTime int64
}

type listEntry struct {
	entries map[*cacheEntry]struct{}
	freq    int
}

func New(c Config) *Cache {
	return &Cache{
		db:      c.DB,
		codec:   c.Codec,
		metrics: c.Metrics,
		prefix:  c.Prefix,
		ttl:     int64(c.TTL.Seconds()),

		values: make(map[string]*cacheEntry),
		freqs:  list.New(),
	}
}

func (c *Cache) increment(e *cacheEntry) {
	e.lastAccessTime = time.Now().Unix()
	currentPlace := e.freqNode
	var nextFreq int
	var nextPlace *list.Element
	if currentPlace == nil {
		// new entry
		nextFreq = 1
		nextPlace = c.freqs.Front()
	} else {
		// move up
		nextFreq = currentPlace.Value.(*listEntry).freq + 1
		nextPlace = currentPlace.Next()
	}

	if nextPlace == nil || nextPlace.Value.(*listEntry).freq != nextFreq {
		// create a new list entry
		li := new(listEntry)
		li.freq = nextFreq
		li.entries = make(map[*cacheEntry]struct{})
		if currentPlace != nil {
			nextPlace = c.freqs.InsertAfter(li, currentPlace)
		} else {
			nextPlace = c.freqs.PushFront(li)
		}
	}
	e.freqNode = nextPlace
	nextPlace.Value.(*listEntry).entries[e] = struct{}{}
	if currentPlace != nil {
		// remove from current position
		c.remEntry(currentPlace, e)
	}
}

func (c *Cache) remEntry(place *list.Element, entry *cacheEntry) {
	entries := place.Value.(*listEntry).entries
	delete(entries, entry)
	if len(entries) == 0 {
		c.freqs.Remove(place)
	}
}

func (c *Cache) Size() uint64 {
	c.lock.Lock()
	defer c.lock.Unlock()
	return uint64(c.len)
}

func (c *Cache) Put(key string, val interface{}) {
	c.lock.Lock()
	c.set(key, val)
	c.lock.Unlock()
}

func (c *Cache) set(key string, value interface{}) {
	e, ok := c.values[key]
	if ok {
		e.value = value
		e.persisted = false
		c.increment(e)
		return
	}
	e = new(cacheEntry)
	e.key = key
	e.value = value
	c.values[key] = e
	c.increment(e)
	c.len++
}

func (c *Cache) Lookup(key string) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	v, err := c.get(key)
	if v == nil || err != nil {
		return nil, false
	}
	return v, true
}

func (c *Cache) GetOrCreate(key string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	v, err := c.get(key)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	v = c.codec.New(key)
	c.set(key, v)
	return v, nil
}

func (c *Cache) get(key string) (interface{}, error) {
	c.metrics.ReadsCounter.Inc()
	e, ok := c.values[key]
	if ok {
		c.increment(e)
		return e.value, nil
	}
	// Value doesn't exist in cache, load from DB.
	c.metrics.MissesCounter.Inc()
	var buf []byte
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
	e = new(cacheEntry)
	e.key = key
	e.value = v
	c.values[key] = e
	c.increment(e)
	c.len++
	return v, nil
}

func (c *Cache) Delete(key string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		c.deleteEntry(e)
	}
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(c.prefix + key))
	})
}

func (c *Cache) Discard(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		c.deleteEntry(e)
	}
}

func (c *Cache) DeletePrefix(prefix string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	for k, e := range c.values {
		if strings.HasPrefix(k, prefix) {
			c.deleteEntry(e)
		}
	}
	return c.db.DropPrefix([]byte(c.prefix + prefix))
}

func (c *Cache) DiscardPrefix(prefix string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for k, e := range c.values {
		if strings.HasPrefix(k, prefix) {
			c.deleteEntry(e)
		}
	}
}

func (c *Cache) deleteEntry(entry *cacheEntry) {
	delete(c.values, entry.key)
	c.remEntry(entry.freqNode, entry)
	c.len--
}

func (c *Cache) Flush() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, err := c.evict(c.len)
	return err
}

// Evict performs cache evictions. The difference between Evict and WriteBack is that evictions happen when cache grows
// above allowed threshold and write-back calls happen constantly, making pyroscope more crash-resilient.
// See https://github.com/pyroscope-io/pyroscope/issues/210 for more context
func (c *Cache) Evict(percent float64) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.EvictionsDuration.Observe))
	defer timer.ObserveDuration()
	return c.evict(int(float64(c.len) * percent))
}

func (c *Cache) evict(count int) (evicted int, err error) {
	batch := c.newWriteBatch()
	defer func() {
		if err == nil && batch.size > 0 {
			err = batch.Flush()
		}
	}()

	for i := 0; i < count; {
		if place := c.freqs.Front(); place != nil {
			for entry := range place.Value.(*listEntry).entries {
				if i >= count {
					return evicted, nil
				}
				if err = batch.write(entry); err != nil {
					return evicted, err
				}
				c.deleteEntry(entry)
				evicted++
				i++
			}
		}
	}

	return evicted, nil
}

func (c *Cache) WriteBack() (persisted, evicted int, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.metrics.WriteBackDuration.Observe))
	defer timer.ObserveDuration()
	batch := c.newWriteBatch()
	defer func() {
		if err == nil && batch.size > 0 {
			err = batch.Flush()
		}
	}()
	now := time.Now().Unix()
	for _, entry := range c.values {
		if !entry.persisted {
			if err = batch.write(entry); err != nil {
				return persisted, evicted, err
			}
			entry.persisted = true
			persisted++
		}
		if c.ttl > 0 && now-entry.lastAccessTime > c.ttl {
			c.deleteEntry(entry)
			evicted++
		}
	}
	return persisted, evicted, nil
}

type writeBatch struct {
	c *Cache
	*badger.WriteBatch

	count int
	size  int
}

func (c *Cache) newWriteBatch() *writeBatch {
	return &writeBatch{c: c, WriteBatch: c.db.NewWriteBatch()}
}

func (b *writeBatch) write(e *cacheEntry) error {
	var buf bytes.Buffer // TODO(kolesnikovae): Use pool with batch?
	if err := b.c.codec.Serialize(&buf, e.key, e.value); err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	k := []byte(b.c.prefix + e.key)
	v := buf.Bytes()
	if b.size+len(v) > int(b.c.db.MaxBatchSize()) || b.count+1 > int(b.c.db.MaxBatchCount()) {
		if err := b.Flush(); err != nil {
			return err
		}
		b.WriteBatch = b.c.db.NewWriteBatch()
		b.size = 0
		b.count = 0
	}
	if err := b.Set(k, v); err != nil {
		return err
	}
	b.size += len(v)
	b.count++
	b.c.metrics.DBWrites.Observe(float64(len(v)))
	return nil
}
