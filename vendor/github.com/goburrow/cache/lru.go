package cache

import (
	"container/list"
)

// lruCache is a LRU cache.
type lruCache struct {
	cache *cache
	cap   int
	ls    list.List
}

// init initializes cache list.
func (l *lruCache) init(c *cache, cap int) {
	l.cache = c
	l.cap = cap
	l.ls.Init()
}

// write adds new entry to the cache and returns evicted entry if necessary.
func (l *lruCache) write(en *entry) *entry {
	// Fast path
	if en.accessList != nil {
		// Entry existed, update its status instead.
		l.markAccess(en)
		return nil
	}
	// Try to add new entry to the list
	cen := l.cache.getOrSet(en)
	if cen == nil {
		// Brand new entry, add to the LRU list.
		en.accessList = l.ls.PushFront(en)
	} else {
		// Entry has already been added, update its value instead.
		cen.setValue(en.getValue())
		cen.setWriteTime(en.getWriteTime())
		if cen.accessList == nil {
			// Entry is loaded to the cache but not yet registered.
			cen.accessList = l.ls.PushFront(cen)
		} else {
			l.markAccess(cen)
		}
	}
	if l.cap > 0 && l.ls.Len() > l.cap {
		// Remove the last element when capacity exceeded.
		en = getEntry(l.ls.Back())
		return l.remove(en)
	}
	return nil
}

// access updates cache entry for a get.
func (l *lruCache) access(en *entry) {
	if en.accessList != nil {
		l.markAccess(en)
	}
}

// markAccess marks the element has just been accessed.
// en.accessList must not be null.
func (l *lruCache) markAccess(en *entry) {
	l.ls.MoveToFront(en.accessList)
}

// remove removes an entry from the cache.
func (l *lruCache) remove(en *entry) *entry {
	if en.accessList == nil {
		// Already deleted
		return nil
	}
	l.cache.delete(en)
	l.ls.Remove(en.accessList)
	en.accessList = nil
	return en
}

// iterate walks through all lists by access time.
func (l *lruCache) iterate(fn func(en *entry) bool) {
	iterateListFromBack(&l.ls, fn)
}

const (
	admissionWindow uint8 = iota
	probationSegment
	protectedSegment
)

const (
	protectedRatio = 0.8
)

// slruCache is a segmented LRU.
// See http://highscalability.com/blog/2016/1/25/design-of-a-modern-cache.html
type slruCache struct {
	cache *cache

	probationCap int
	probationLs  list.List

	protectedCap int
	protectedLs  list.List
}

// init initializes the cache list.
func (l *slruCache) init(c *cache, cap int) {
	l.cache = c
	l.protectedCap = int(float64(cap) * protectedRatio)
	l.probationCap = cap - l.protectedCap
	l.probationLs.Init()
	l.protectedLs.Init()
}

// length returns total number of entries in the cache.
func (l *slruCache) length() int {
	return l.probationLs.Len() + l.protectedLs.Len()
}

// write adds new entry to the cache and returns evicted entry if necessary.
func (l *slruCache) write(en *entry) *entry {
	// Fast path
	if en.accessList != nil {
		// Entry existed, update its value instead.
		l.markAccess(en)
		return nil
	}
	// Try to add new entry to the probation segment.
	cen := l.cache.getOrSet(en)
	if cen == nil {
		// Brand new entry, add to the probation segment.
		en.listID = probationSegment
		en.accessList = l.probationLs.PushFront(en)
	} else {
		// Entry has already been added, update its value instead.
		cen.setValue(en.getValue())
		cen.setWriteTime(en.getWriteTime())
		if cen.accessList == nil {
			// Entry is loaded to the cache but not yet registered.
			cen.listID = probationSegment
			cen.accessList = l.probationLs.PushFront(cen)
		} else {
			l.markAccess(cen)
		}
	}
	// The probation list can exceed its capacity if number of entries
	// is still under total allowed capacity.
	if l.probationCap > 0 && l.probationLs.Len() > l.probationCap &&
		l.length() > (l.probationCap+l.protectedCap) {
		// Remove the last element when capacity exceeded.
		en = getEntry(l.probationLs.Back())
		return l.remove(en)
	}
	return nil
}

// access updates cache entry for a get.
func (l *slruCache) access(en *entry) {
	if en.accessList != nil {
		l.markAccess(en)
	}
}

// markAccess marks the element has just been accessed.
// en.accessList must not be null.
func (l *slruCache) markAccess(en *entry) {
	if en.listID == protectedSegment {
		// Already in the protected segment.
		l.protectedLs.MoveToFront(en.accessList)
		return
	}
	// The entry is currently in the probation segment, promote it to the protected segment.
	en.listID = protectedSegment
	l.probationLs.Remove(en.accessList)
	en.accessList = l.protectedLs.PushFront(en)

	if l.protectedCap > 0 && l.protectedLs.Len() > l.protectedCap {
		// Protected list capacity exceeded, move the last entry in the protected segment to
		// the probation segment.
		en = getEntry(l.protectedLs.Back())
		en.listID = probationSegment
		l.protectedLs.Remove(en.accessList)
		en.accessList = l.probationLs.PushFront(en)
	}
}

// remove removes an entry from the cache and returns the removed entry or nil
// if it is not found.
func (l *slruCache) remove(en *entry) *entry {
	if en.accessList == nil {
		return nil
	}
	l.cache.delete(en)
	if en.listID == protectedSegment {
		l.protectedLs.Remove(en.accessList)
	} else {
		l.probationLs.Remove(en.accessList)
	}
	en.accessList = nil
	return en
}

// victim returns the last entry in probation list if total entries reached the limit.
func (l *slruCache) victim() *entry {
	if l.probationCap <= 0 || l.length() < (l.probationCap+l.protectedCap) {
		return nil
	}
	el := l.probationLs.Back()
	if el == nil {
		return nil
	}
	return getEntry(el)
}

// iterate walks through all lists by access time.
func (l *slruCache) iterate(fn func(en *entry) bool) {
	iterateListFromBack(&l.protectedLs, fn)
	iterateListFromBack(&l.probationLs, fn)
}
