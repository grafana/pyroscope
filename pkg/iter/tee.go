package iter

import (
	"math"
	"sync"
)

const defaultTeeBufferSize = 512

// Tee returns n independent iterators from a single iterable.
//
// The original iterator should not be used anywhere else, except that it's
// caller responsibility to close it and handle the error.
//
// Tee buffers source objects, and frees them eventually: when an object
// from the source iterator is consumed, the ownership is transferred to Tee.
// Therefore, the caller must ensure the source iterator never reuses objects
// returned with At.
//
// Tee never blocks the leader iterator, instead, it grows the internal buffer:
// if any of the returned iterators are abandoned, all source iterator objects
// will be held in the buffer.
func Tee[T any](iter Iterator[T], n int) []Iterator[T] {
	return newTee[T](iter, n, defaultTeeBufferSize)
}

func newTee[T any](iter Iterator[T], n, bufSize int) []Iterator[T] {
	if n < 2 {
		return []Iterator[T]{iter}
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
			s: s,
			n: i,
		}
	}
	return t
}

type sharedIterator[T any] struct {
	s int64
	i Iterator[T]
	e error
	t []int64
	m sync.RWMutex
	v []T
	w int64
}

func (s *sharedIterator[T]) next(n int) bool {
	s.m.RLock()
	if p := s.t[n]; p < s.w {
		s.t[n]++
		s.m.RUnlock()
		return true
	}
	s.m.RUnlock()
	s.m.Lock()
	defer s.m.Unlock()
	if p := s.t[n]; p < s.w {
		s.t[n]++
		return true
	}
	// All the memoized items were consumed.
	// If the source iterator has errored, stop here.
	if s.e != nil {
		return false
	}
	s.clean() // Conditionally clean consumed values.
	// Fetch the next batch from the source iterator.
	var i int64
	for ; i < s.s; i++ {
		if !s.i.Next() {
			break
		}
		s.v = append(s.v, s.i.At())
	}
	s.e = s.i.Err()
	s.w += i
	if i != 0 {
		s.t[n]++
		return true
	}
	return false
}

func (s *sharedIterator[T]) clean() {
	lo := int64(-1)
	for _, v := range s.t {
		if v < lo || lo == -1 {
			lo = v
		}
	}
	// There is no much sense to trim the
	// buffer, if less than the initial
	// capacity will be freed.
	if lo < s.s {
		return
	}
	if lo == math.MaxInt64 {
		// All iterators have been closed.
		return
	}
	// Clean values that will be removed.
	var v T
	for i := range s.v[:lo] {
		s.v[i] = v
	}
	// Shift remaining values to the beginning.
	s.v = s.v[:copy(s.v, s.v[lo:])]
	// Update offsets accordingly.
	s.w -= lo
	for i := range s.t {
		if s.t[i] != math.MaxInt64 {
			s.t[i] -= lo
		}
	}
}

func (s *sharedIterator[T]) at(n int) T {
	s.m.RLock()
	v := s.v[s.t[n]]
	s.m.RUnlock()
	return v
}

func (s *sharedIterator[T]) close(n int) {
	s.m.RLock()
	s.t[n] = math.MaxInt64
	s.m.RUnlock()
}

func (s *sharedIterator[T]) err() error {
	s.m.RLock()
	e := s.e
	s.m.RUnlock()
	return e
}

type tee[T any] struct {
	s *sharedIterator[T]
	n int
}

func (t *tee[T]) Next() bool { return t.s.next(t.n) }

func (t *tee[T]) At() T { return t.s.at(t.n) }

func (t *tee[T]) Err() error { return t.s.err() }

func (t *tee[T]) Close() error {
	t.s.close(t.n)
	return nil
}
