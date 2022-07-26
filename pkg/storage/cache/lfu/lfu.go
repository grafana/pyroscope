package lfu

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	TTL              int64
	EvictionChannel  chan<- Eviction
	WriteBackChannel chan<- Eviction

	lock   sync.Mutex
	values map[string]*cacheEntry
	freqs  *list.List
	len    int
}

type Eviction struct {
	Key   string
	Value interface{}
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

func New() *Cache {
	return &Cache{
		values: make(map[string]*cacheEntry),
		freqs:  list.New(),
	}
}

func (c *Cache) Get(key string) interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		c.increment(e)
		return e.value
	}
	return nil
}

func (c *Cache) GetOrSet(key string, value func() (interface{}, error)) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	e, ok := c.values[key]
	if ok {
		c.increment(e)
		return e.value, nil
	}
	// value doesn't exist.
	v, err := value()
	if err != nil || v == nil {
		return nil, err
	}
	e = new(cacheEntry)
	e.key = key
	e.value = v
	// The item returned by value() is either newly allocated or was just
	// read from the DB, therefore we mark it as persisted to avoid redundant
	// writes or writing empty object. Once the item is invalidated, caller
	// has to explicitly set it with Set call.
	e.persisted = true
	c.values[key] = e
	c.increment(e)
	c.len++
	return v, nil
}

func (c *Cache) Set(key string, value interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		// value already exists for key.  overwrite
		e.value = value
		e.persisted = false
		c.increment(e)
	} else {
		// value doesn't exist.  insert
		e = new(cacheEntry)
		e.key = key
		e.value = value
		c.values[key] = e
		c.increment(e)
		c.len++
	}
}

func (c *Cache) Delete(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if e, ok := c.values[key]; ok {
		c.delete(e)
	}
}

func (c *Cache) DeletePrefix(prefix string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for k, e := range c.values {
		if strings.HasPrefix(k, prefix) {
			c.delete(e)
		}
	}
}

//revive:disable-next-line:confusing-naming methods are different
func (c *Cache) delete(entry *cacheEntry) {
	delete(c.values, entry.key)
	c.remEntry(entry.freqNode, entry)
	c.len--
}

func (c *Cache) Len() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.len
}

func (c *Cache) Evict(count int) int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.evict(count)
}

// WriteBack persists modified items and evicts obsolete ones.
func (c *Cache) WriteBack() (persisted, evicted int) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.writeBack()
}

func (c *Cache) Iterate(fn func(k string, v interface{}) error) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	for k, entry := range c.values {
		if err := fn(k, entry.value); err != nil {
			return err
		}
	}
	return nil
}

//revive:disable-next-line:confusing-naming methods are different
func (c *Cache) evict(count int) int {
	// No lock here so it can be called
	// from within the lock (during Set)
	var evicted int
	for i := 0; i < count; {
		if place := c.freqs.Front(); place != nil {
			for entry := range place.Value.(*listEntry).entries {
				if i >= count {
					return evicted
				}
				if c.EvictionChannel != nil && !entry.persisted {
					c.EvictionChannel <- Eviction{
						Key:   entry.key,
						Value: entry.value,
					}
				}
				c.delete(entry)
				evicted++
				i++
			}
		}
	}
	return evicted
}

//revive:disable-next-line:confusing-naming methods are different
func (c *Cache) writeBack() (persisted, evicted int) {
	now := time.Now().Unix()
	for k, entry := range c.values {
		if c.WriteBackChannel != nil && !entry.persisted {
			c.WriteBackChannel <- Eviction{
				Key:   k,
				Value: entry.value,
			}
			entry.persisted = true
			persisted++
		}
		if c.TTL > 0 && now-entry.lastAccessTime > c.TTL {
			c.delete(entry)
			evicted++
		}
	}
	return persisted, evicted
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
