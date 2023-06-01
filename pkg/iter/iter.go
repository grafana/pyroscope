package iter

import (
	"sort"

	"github.com/samber/lo"
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

func Slice[T any](it Iterator[T]) ([]T, error) {
	var result []T
	defer it.Close()
	for it.Next() {
		result = append(result, it.At())
	}
	return result, it.Err()
}

// CloneN returns N copy of the iterator.
// The returned iterators are independent of the original iterator.
// The original might be exhausted and should be discarded.
func CloneN[T any](it Iterator[T], n int) ([]Iterator[T], error) {
	if sl, ok := it.(*sliceIterator[T]); ok {
		return lo.Times(n, func(_ int) Iterator[T] { return NewSliceIterator(sl.list) }), nil
	}
	slice, err := Slice(it)
	if err != nil {
		return nil, err
	}
	return lo.Times(n, func(_ int) Iterator[T] { return NewSliceIterator(slice) }), nil
}

type unionIterator[T any] struct {
	iters []Iterator[T]
}

func NewUnionIterator[T any](iters ...Iterator[T]) Iterator[T] {
	return &unionIterator[T]{
		iters: iters,
	}
}

func (u *unionIterator[T]) Next() bool {
	idx := 0
	for idx < len(u.iters) {
		it := u.iters[idx]

		if it.Next() {
			return true
		}
		if it.Err() != nil {
			return false
		}

		u.iters = u.iters[1:]
	}
	return false
}

func (it *unionIterator[T]) At() T {
	return it.iters[0].At()
}

func (it *unionIterator[T]) Err() error {
	return it.iters[0].Err()
}

func (it *unionIterator[T]) Close() error {
	for _, it := range it.iters {
		if err := it.Close(); err != nil {
			return err
		}
	}
	return nil
}

type emptyIterator[T any] struct{}

func NewEmptyIterator[T any]() Iterator[T] {
	return &emptyIterator[T]{}
}

func (it *emptyIterator[T]) Next() bool {
	return false
}

func (it *emptyIterator[T]) At() T {
	var t T
	return t
}

func (it *emptyIterator[T]) Err() error {
	return nil
}

func (it *emptyIterator[T]) Close() error {
	return nil
}

type BufferedIterator[T any] struct {
	Iterator[T]
	buff chan T
	at   T
}

// NewBufferedIterator returns an iterator that reads asynchronously from the given iterator and buffers up to size elements.
func NewBufferedIterator[T any](it Iterator[T], size int) Iterator[T] {
	buffered := &BufferedIterator[T]{
		Iterator: it,
		buff:     make(chan T, size),
	}
	go buffered.fill()
	return buffered
}

func (it *BufferedIterator[T]) fill() {
	defer close(it.buff)
	for it.Iterator.Next() {
		it.buff <- it.Iterator.At()
	}
}

func (it *BufferedIterator[T]) Next() bool {
	at, ok := <-it.buff
	if ok {
		it.at = at
	}
	return ok
}

func (it *BufferedIterator[T]) At() T {
	return it.at
}
