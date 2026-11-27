package iter

import (
	"math"
	"sync"
)

const (
	defaultTeeBufferSize = 4096
	teeLocalBatchSize    = 128
)

// Tee returns 2 independent iterators from a single iterable.
//
// The original iterator should not be used anywhere else, except that it's
// caller responsibility to close it and handle the error, after all the
// tee iterators finished.
//
// Tee buffers source objects, and frees them eventually: when an object
// from the source iterator is consumed, the ownership is transferred to Tee.
// Therefore, the caller must ensure the source iterator never reuses objects
// returned with At.
//
// Tee never blocks the leader iterator, instead, it grows the internal buffer:
// if any of the returned iterators are abandoned, all source iterator objects
// will be held in the buffer.
func Tee[T any](iter Iterator[T]) (a, b Iterator[T]) {
	s := newTee[T](iter, 2, defaultTeeBufferSize)
	return s[0], s[1]
}

func TeeN[T any](iter Iterator[T], n int) []Iterator[T] {
	return newTee[T](iter, n, defaultTeeBufferSize)
}

// NOTE(kolesnikovae): The implementation design aims for simplicity.
// A more efficient tee can be implemented on top of a linked
// list of small arrays:
//  - More efficient (de-)allocations (chunk pool).
//  - Less/no mutex contention.

func newTee[T any](iter Iterator[T], n, bufSize int) []Iterator[T] {
	if n < 0 {
		return nil
	}
	s := &sharedIterator[T]{
		s: int64(bufSize),
		i: iter,
		t: make([]int64, n),
		v: make([]T, 0, bufSize),
	}
	t := make([]Iterator[T], n)
	for i := range s.t {
		t[i] = &tee[T]{
			s:       s,
			n:       i,
			scratch: make([]T, min(bufSize, teeLocalBatchSize)),
		}
	}
	return t
}

type sharedIterator[T any] struct {
	s int64
	i Iterator[T]
	e error
	t []int64
	m sync.Mutex
	v []T
	w int64
}

// nextBatch copies into consumer-owned storage so clean can safely move the
// shared buffer while other consumers read their local batches.
func (s *sharedIterator[T]) nextBatch(t *tee[T]) bool {
	s.m.Lock()
	defer s.m.Unlock()
	if s.t[t.n] == math.MaxInt64 {
		return false
	}
	if s.t[t.n] == s.w {
		if s.e != nil {
			return false
		}
		s.clean()
		var i int64
		for ; i < s.s; i++ {
			if !s.i.Next() {
				break
			}
			s.v = append(s.v, s.i.At())
		}
		s.e = s.i.Err()
		s.w += i
		if i == 0 {
			return false
		}
	}
	n := copy(t.scratch, s.v[s.t[t.n]:s.w])
	s.t[t.n] += int64(n)
	t.buf = t.scratch[:n]
	// Offsets track values copied out, not values returned by Next.
	s.clean()
	return n > 0
}

func (s *sharedIterator[T]) clean() {
	lo := int64(-1)
	for _, v := range s.t {
		if v < lo || lo == -1 {
			lo = v
		}
	}
	if lo < s.s {
		return
	}
	if lo == math.MaxInt64 {
		// All iterators have been closed.
		return
	}
	// Shift remaining values and clear the vacated tail, including references
	// beyond the new length of the shared buffer.
	n := copy(s.v, s.v[lo:])
	clear(s.v[n:])
	s.v = s.v[:n]
	s.w -= lo
	for i := range s.t {
		if s.t[i] != math.MaxInt64 {
			s.t[i] -= lo
		}
	}
}

func (s *sharedIterator[T]) close(n int) {
	s.m.Lock()
	s.t[n] = math.MaxInt64
	s.m.Unlock()
}

func (s *sharedIterator[T]) err() error {
	s.m.Lock()
	e := s.e
	s.m.Unlock()
	return e
}

// Each consumer is used by a single goroutine, independently of other consumers.
type tee[T any] struct {
	s       *sharedIterator[T]
	n       int
	scratch []T
	buf     []T
	cur     T
}

func (t *tee[T]) Next() bool {
	if len(t.buf) == 0 {
		var zero T
		t.cur = zero
		t.buf = nil
		if !t.s.nextBatch(t) {
			return false
		}
	}
	t.cur = t.buf[0]
	var zero T
	t.buf[0] = zero
	t.buf = t.buf[1:]
	return true
}

func (t *tee[T]) At() T { return t.cur }

func (t *tee[T]) Err() error { return t.s.err() }

func (t *tee[T]) Close() error {
	clear(t.scratch)
	t.buf = nil
	var zero T
	t.cur = zero
	t.s.close(t.n)
	return nil
}
