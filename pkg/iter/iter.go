package iter

import (
	"sort"

	"golang.org/x/exp/constraints"
)

type Iterator[A any] interface {
	// Next advances the iterator and returns true if another value was found.
	Next() bool

	// At returns the value at the current iterator position.
	At() A

	// Err returns the last error of the iterator.
	Err() error

	Close() error
}

type SeekIterator[A any, B any] interface {
	Iterator[A]

	// Like Next but skips over results until reading >= the given location
	Seek(pos B) bool
}

type errIterator[A any] struct {
	err error
}

func NewErrIterator[A any](err error) Iterator[A] {
	return &errIterator[A]{
		err: err,
	}
}

func (i *errIterator[A]) Err() error {
	return i.err
}
func (*errIterator[A]) At() (a A) {
	return a
}
func (*errIterator[A]) Next() bool {
	return false
}

func (*errIterator[A]) Close() error {
	return nil
}

type errSeekIterator[A any, B any] struct {
	Iterator[A]
}

func NewErrSeekIterator[A any, B any](err error) SeekIterator[A, B] {
	return &errSeekIterator[A, B]{
		Iterator: NewErrIterator[A](err),
	}
}

func (*errSeekIterator[A, B]) Seek(_ B) bool {
	return false
}

type sliceIterator[A any] struct {
	list []A
	cur  A
}

func NewSliceIterator[A any](s []A) Iterator[A] {
	return &sliceIterator[A]{
		list: s,
	}
}

func (i *sliceIterator[A]) Err() error {
	return nil
}
func (i *sliceIterator[A]) Next() bool {
	if len(i.list) > 0 {
		i.cur = i.list[0]
		i.list = i.list[1:]
		return true
	}
	var a A
	i.cur = a
	return false
}

func NewSliceSeekIterator[A constraints.Ordered](s []A) SeekIterator[A, A] {
	return &sliceSeekIterator[A]{
		sliceIterator: &sliceIterator[A]{
			list: s,
		},
	}
}

type sliceSeekIterator[A constraints.Ordered] struct {
	*sliceIterator[A]
}

func (i *sliceSeekIterator[A]) Seek(x A) bool {
	// If the current value satisfies, then return.
	if i.cur >= x {
		return true
	}
	if len(i.list) == 0 {
		return false
	}

	// Do binary search between current position and end.
	pos := sort.Search(len(i.list), func(pos int) bool {
		return i.list[pos] >= x
	})
	if pos < len(i.list) {
		i.cur = i.list[pos]
		i.list = i.list[pos+1:]
		return true
	}
	i.list = nil
	return false
}

func (i *sliceIterator[A]) At() A {
	return i.cur
}

func (i *sliceIterator[A]) Close() error {
	return nil
}
