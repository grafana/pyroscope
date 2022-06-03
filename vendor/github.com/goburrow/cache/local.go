package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// Default maximum number of cache entries.
	maximumCapacity = 1 << 30
	// Buffer size of entry channels
	chanBufSize = 64
	// Maximum number of entries to be drained in a single clean up.
	drainMax = 16
	// Number of cache access operations that will trigger clean up.
	drainThreshold = 64
)

// currentTime is an alias for time.Now, used for testing.
var currentTime = time.Now

// localCache is an asynchronous LRU cache.
type localCache struct {
	// internal data structure
	cache cache // Must be aligned on 32-bit

	// user configurations
	expireAfterAccess time.Duration
	expireAfterWrite  time.Duration
	refreshAfterWrite time.Duration
	policyName        string

	onInsertion Func
	onRemoval   Func

	loader   LoaderFunc
	reloader Reloader
	stats    StatsCounter

	// cap is the cache capacity.
	cap int

	// accessQueue is the cache retention policy, which manages entries by access time.
	accessQueue policy
	// writeQueue is for managing entries by write time.
	// It is only fulfilled when expireAfterWrite or refreshAfterWrite is set.
	writeQueue policy
	// events is the cache event queue for processEntries
	events chan entryEvent

	// readCount is a counter of the number of reads since the last write.
	readCount int32

	// for closing routines created by this cache.
	closing int32
	closeWG sync.WaitGroup
}

// newLocalCache returns a default localCache.
// init must be called before this cache can be used.
func newLocalCache() *localCache {
	return &localCache{
		cap:   maximumCapacity,
		cache: cache{},
		stats: &statsCounter{},
	}
}

// init initializes cache replacement policy after all user configuration properties are set.
func (c *localCache) init() {
	c.accessQueue = newPolicy(c.policyName)
	c.accessQueue.init(&c.cache, c.cap)
	if c.expireAfterWrite > 0 || c.refreshAfterWrite > 0 {
		c.writeQueue = &recencyQueue{}
	} else {
		c.writeQueue = discardingQueue{}
	}
	c.writeQueue.init(&c.cache, c.cap)
	c.events = make(chan entryEvent, chanBufSize)

	c.closeWG.Add(1)
	go c.processEntries()
}

// Close implements io.Closer and always returns a nil error.
// Caller would ensure the cache is not being used (reading and writing) before closing.
func (c *localCache) Close() error {
	if atomic.CompareAndSwapInt32(&c.closing, 0, 1) {
		// Do not close events channel to avoid panic when cache is still being used.
		c.events <- entryEvent{nil, eventClose}
		// Wait for the goroutine to close this channel
		c.closeWG.Wait()
	}
	return nil
}

// GetIfPresent gets cached value from entries list and updates
// last access time for the entry if it is found.
func (c *localCache) GetIfPresent(k Key) (Value, bool) {
	en := c.cache.get(k, sum(k))
	if en == nil {
		c.stats.RecordMisses(1)
		return nil, false
	}
	now := currentTime()
	if c.isExpired(en, now) {
		c.sendEvent(eventDelete, en)
		c.stats.RecordMisses(1)
		return nil, false
	}
	c.setEntryAccessTime(en, now)
	c.sendEvent(eventAccess, en)
	c.stats.RecordHits(1)
	return en.getValue(), true
}

// Put adds new entry to entries list.
func (c *localCache) Put(k Key, v Value) {
	h := sum(k)
	en := c.cache.get(k, h)
	now := currentTime()
	if en == nil {
		en = newEntry(k, v, h)
		c.setEntryWriteTime(en, now)
		c.setEntryAccessTime(en, now)
		// Add to the cache directly so the new value is available immediately.
		// However, only do this within the cache capacity (approximately).
		if c.cap == 0 || c.cache.len() < c.cap {
			cen := c.cache.getOrSet(en)
			if cen != nil {
				cen.setValue(v)
				c.setEntryWriteTime(cen, now)
				en = cen
			}
		}
	} else {
		// Update value and send notice
		en.setValue(v)
		c.setEntryWriteTime(en, now)
	}
	c.sendEvent(eventWrite, en)
}

// Invalidate removes the entry associated with key k.
func (c *localCache) Invalidate(k Key) {
	en := c.cache.get(k, sum(k))
	if en != nil {
		en.setInvalidated(true)
		c.sendEvent(eventDelete, en)
	}
}

// InvalidateAll resets entries list.
func (c *localCache) InvalidateAll() {
	c.cache.walk(func(en *entry) {
		en.setInvalidated(true)
	})
	c.sendEvent(eventDelete, nil)
}

// Get returns value associated with k or call underlying loader to retrieve value
// if it is not in the cache. The returned value is only cached when loader returns
// nil error.
func (c *localCache) Get(k Key) (Value, error) {
	en := c.cache.get(k, sum(k))
	if en == nil {
		c.stats.RecordMisses(1)
		return c.load(k)
	}
	// Check if this entry needs to be refreshed
	now := currentTime()
	if c.isExpired(en, now) {
		if c.loader == nil {
			c.sendEvent(eventDelete, en)
		} else {
			// For loading cache, we do not delete entry but leave it to
			// the eviction policy, so users still can get the old value.
			c.setEntryAccessTime(en, now)
			c.refreshAsync(en)
		}
		c.stats.RecordMisses(1)
	} else {
		c.setEntryAccessTime(en, now)
		c.sendEvent(eventAccess, en)
		c.stats.RecordHits(1)
	}
	return en.getValue(), nil
}

// Refresh asynchronously reloads value for Key if it existed, otherwise
// it will synchronously load and block until it value is loaded.
func (c *localCache) Refresh(k Key) {
	if c.loader == nil {
		return
	}
	en := c.cache.get(k, sum(k))
	if en == nil {
		c.load(k)
	} else {
		c.refreshAsync(en)
	}
}

// Stats copies cache stats to t.
func (c *localCache) Stats(t *Stats) {
	c.stats.Snapshot(t)
}

func (c *localCache) processEntries() {
	defer c.closeWG.Done()
	for e := range c.events {
		switch e.event {
		case eventWrite:
			c.write(e.entry)
			c.postWriteCleanup()
		case eventAccess:
			c.access(e.entry)
			c.postReadCleanup()
		case eventDelete:
			if e.entry == nil {
				c.removeAll()
			} else {
				c.remove(e.entry)
			}
			c.postReadCleanup()
		case eventClose:
			if c.reloader != nil {
				// Stop all refresh tasks.
				c.reloader.Close()
			}
			c.removeAll()
			return
		}
	}
}

// sendEvent sends event only when the cache is not closing/closed.
func (c *localCache) sendEvent(typ event, en *entry) {
	if atomic.LoadInt32(&c.closing) == 0 {
		c.events <- entryEvent{en, typ}
	}
}

// This function must only be called from processEntries goroutine.
func (c *localCache) write(en *entry) {
	ren := c.accessQueue.write(en)
	c.writeQueue.write(en)
	if c.onInsertion != nil {
		c.onInsertion(en.key, en.getValue())
	}
	if ren != nil {
		c.writeQueue.remove(ren)
		// An entry has been evicted
		c.stats.RecordEviction()
		if c.onRemoval != nil {
			c.onRemoval(ren.key, ren.getValue())
		}
	}
}

// removeAll remove all entries in the cache.
// This function must only be called from processEntries goroutine.
func (c *localCache) removeAll() {
	c.accessQueue.iterate(func(en *entry) bool {
		c.remove(en)
		return true
	})
}

// remove removes the given element from the cache and entries list.
// It also calls onRemoval callback if it is set.
func (c *localCache) remove(en *entry) {
	ren := c.accessQueue.remove(en)
	c.writeQueue.remove(en)
	if ren != nil && c.onRemoval != nil {
		c.onRemoval(ren.key, ren.getValue())
	}
}

// access moves the given element to the top of the entries list.
// This function must only be called from processEntries goroutine.
func (c *localCache) access(en *entry) {
	c.accessQueue.access(en)
}

// load uses current loader to synchronously retrieve value for k and adds new
// entry to the cache only if loader returns a nil error.
func (c *localCache) load(k Key) (Value, error) {
	if c.loader == nil {
		panic("cache loader function must be set")
	}
	// TODO: Poll the value instead when the entry is loading.
	start := currentTime()
	v, err := c.loader(k)
	now := currentTime()
	loadTime := now.Sub(start)
	if err != nil {
		c.stats.RecordLoadError(loadTime)
		return nil, err
	}
	en := newEntry(k, v, sum(k))
	c.setEntryWriteTime(en, now)
	c.setEntryAccessTime(en, now)
	if c.cap == 0 || c.cache.len() < c.cap {
		cen := c.cache.getOrSet(en)
		if cen != nil {
			cen.setValue(v)
			c.setEntryWriteTime(cen, now)
			en = cen
		}
	}
	c.sendEvent(eventWrite, en)
	c.stats.RecordLoadSuccess(loadTime)
	return v, nil
}

// refreshAsync reloads value in a go routine or using custom executor if defined.
func (c *localCache) refreshAsync(en *entry) bool {
	if en.setLoading(true) {
		// Only do refresh if it isn't running.
		if c.reloader == nil {
			go c.refresh(en)
		} else {
			c.reload(en)
		}
		return true
	}
	return false
}

// refresh reloads value for the given key. If loader returns an error,
// that error will be omitted. Otherwise, the entry value will be updated.
// This function would only be called by refreshAsync.
func (c *localCache) refresh(en *entry) {
	defer en.setLoading(false)

	start := currentTime()
	v, err := c.loader(en.key)
	now := currentTime()
	loadTime := now.Sub(start)
	if err == nil {
		en.setValue(v)
		c.setEntryWriteTime(en, now)
		c.sendEvent(eventWrite, en)
		c.stats.RecordLoadSuccess(loadTime)
	} else {
		// TODO: Log error
		c.stats.RecordLoadError(loadTime)
	}
}

// reload uses user-defined reloader to reloads value.
func (c *localCache) reload(en *entry) {
	start := currentTime()
	setFn := func(newValue Value, err error) {
		defer en.setLoading(false)
		now := currentTime()
		loadTime := now.Sub(start)
		if err == nil {
			en.setValue(newValue)
			c.setEntryWriteTime(en, now)
			c.sendEvent(eventWrite, en)
			c.stats.RecordLoadSuccess(loadTime)
		} else {
			c.stats.RecordLoadError(loadTime)
		}
	}
	c.reloader.Reload(en.key, en.getValue(), setFn)
}

// postReadCleanup is run after entry access/delete event.
// This function must only be called from processEntries goroutine.
func (c *localCache) postReadCleanup() {
	if atomic.AddInt32(&c.readCount, 1) > drainThreshold {
		atomic.StoreInt32(&c.readCount, 0)
		c.expireEntries()
	}
}

// postWriteCleanup is run after entry add event.
// This function must only be called from processEntries goroutine.
func (c *localCache) postWriteCleanup() {
	atomic.StoreInt32(&c.readCount, 0)
	c.expireEntries()
}

// expireEntries removes expired entries.
func (c *localCache) expireEntries() {
	remain := drainMax
	now := currentTime()
	if c.expireAfterAccess > 0 {
		expiry := now.Add(-c.expireAfterAccess).UnixNano()
		c.accessQueue.iterate(func(en *entry) bool {
			if remain == 0 || en.getAccessTime() >= expiry {
				// Can stop as the entries are sorted by access time.
				// (the next entry is accessed more recently.)
				return false
			}
			// accessTime + expiry passed
			c.remove(en)
			c.stats.RecordEviction()
			remain--
			return remain > 0
		})
	}
	if remain > 0 && c.expireAfterWrite > 0 {
		expiry := now.Add(-c.expireAfterWrite).UnixNano()
		c.writeQueue.iterate(func(en *entry) bool {
			if remain == 0 || en.getWriteTime() >= expiry {
				return false
			}
			// writeTime + expiry passed
			c.remove(en)
			c.stats.RecordEviction()
			remain--
			return remain > 0
		})
	}
	if remain > 0 && c.loader != nil && c.refreshAfterWrite > 0 {
		expiry := now.Add(-c.refreshAfterWrite).UnixNano()
		c.writeQueue.iterate(func(en *entry) bool {
			if remain == 0 || en.getWriteTime() >= expiry {
				return false
			}
			// FIXME: This can cause deadlock if the custom executor runs refresh in current go routine.
			// The refresh function, when finish, will send to event channels.
			if c.refreshAsync(en) {
				// TODO: Maybe move this entry up?
				remain--
			}
			return remain > 0
		})
	}
}

func (c *localCache) isExpired(en *entry, now time.Time) bool {
	if en.getInvalidated() {
		return true
	}
	if c.expireAfterAccess > 0 && en.getAccessTime() < now.Add(-c.expireAfterAccess).UnixNano() {
		// accessTime + expiry passed
		return true
	}
	if c.expireAfterWrite > 0 && en.getWriteTime() < now.Add(-c.expireAfterWrite).UnixNano() {
		// writeTime + expiry passed
		return true
	}
	return false
}

func (c *localCache) needRefresh(en *entry, now time.Time) bool {
	if en.getLoading() {
		return false
	}
	if c.refreshAfterWrite > 0 {
		tm := en.getWriteTime()
		if tm > 0 && tm < now.Add(-c.refreshAfterWrite).UnixNano() {
			// writeTime + refresh passed
			return true
		}
	}
	return false
}

// setEntryAccessTime sets access time if needed.
func (c *localCache) setEntryAccessTime(en *entry, now time.Time) {
	if c.expireAfterAccess > 0 {
		en.setAccessTime(now.UnixNano())
	}
}

// setEntryWriteTime sets write time if needed.
func (c *localCache) setEntryWriteTime(en *entry, now time.Time) {
	if c.expireAfterWrite > 0 || c.refreshAfterWrite > 0 {
		en.setWriteTime(now.UnixNano())
	}
}

// New returns a local in-memory Cache.
func New(options ...Option) Cache {
	c := newLocalCache()
	for _, opt := range options {
		opt(c)
	}
	c.init()
	return c
}

// NewLoadingCache returns a new LoadingCache with given loader function
// and cache options.
func NewLoadingCache(loader LoaderFunc, options ...Option) LoadingCache {
	c := newLocalCache()
	c.loader = loader
	for _, opt := range options {
		opt(c)
	}
	c.init()
	return c
}

// Option add options for default Cache.
type Option func(c *localCache)

// WithMaximumSize returns an Option which sets maximum size for the cache.
// Any non-positive numbers is considered as unlimited.
func WithMaximumSize(size int) Option {
	if size < 0 {
		size = 0
	}
	if size > maximumCapacity {
		size = maximumCapacity
	}
	return func(c *localCache) {
		c.cap = size
	}
}

// WithRemovalListener returns an Option to set cache to call onRemoval for each
// entry evicted from the cache.
func WithRemovalListener(onRemoval Func) Option {
	return func(c *localCache) {
		c.onRemoval = onRemoval
	}
}

// WithExpireAfterAccess returns an option to expire a cache entry after the
// given duration without being accessed.
func WithExpireAfterAccess(d time.Duration) Option {
	return func(c *localCache) {
		c.expireAfterAccess = d
	}
}

// WithExpireAfterWrite returns an option to expire a cache entry after the
// given duration from creation.
func WithExpireAfterWrite(d time.Duration) Option {
	return func(c *localCache) {
		c.expireAfterWrite = d
	}
}

// WithRefreshAfterWrite returns an option to refresh a cache entry after the
// given duration. This option is only applicable for LoadingCache.
func WithRefreshAfterWrite(d time.Duration) Option {
	return func(c *localCache) {
		c.refreshAfterWrite = d
	}
}

// WithStatsCounter returns an option which overrides default cache stats counter.
func WithStatsCounter(st StatsCounter) Option {
	return func(c *localCache) {
		c.stats = st
	}
}

// WithPolicy returns an option which sets cache policy associated to the given name.
// Supported policies are: lru, slru, tinylfu.
func WithPolicy(name string) Option {
	return func(c *localCache) {
		c.policyName = name
	}
}

// WithReloader returns an option which sets reloader for a loading cache.
// By default, each asynchronous reload is run in a go routine.
// This option is only applicable for LoadingCache.
func WithReloader(reloader Reloader) Option {
	return func(c *localCache) {
		c.reloader = reloader
	}
}

// withInsertionListener is used for testing.
func withInsertionListener(onInsertion Func) Option {
	return func(c *localCache) {
		c.onInsertion = onInsertion
	}
}
