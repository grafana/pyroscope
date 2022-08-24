package firedb

type Iterator[T any] interface {
	Next() (T, bool)
	Err() error
}

type errIterator[T any] struct {
	err error
}

func (i *errIterator[T]) Err() error {
	return i.err
}
func (*errIterator[T]) Next() (T, bool) {
	var t T
	return t, false
}

func NewErrIterator[T any](err error) Iterator[T] {
	return &errIterator[T]{
		err: err,
	}
}

type sliceIterator[T any] struct {
	s   []T
	pos int
}

func (i *sliceIterator[T]) Err() error {
	return nil
}
func (i *sliceIterator[T]) Next() (T, bool) {
	if i.pos >= len(i.s) {
		var t T
		return t, false
	}
	i.pos++
	return i.s[i.pos-1], true
}

func NewSliceIterator[T any](s []T) Iterator[T] {
	return &sliceIterator[T]{
		s: s,
	}
}
