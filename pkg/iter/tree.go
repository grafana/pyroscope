package iter

import (
	"github.com/grafana/phlare/pkg/util/loser"
)

var _ Iterator[interface{}] = &TreeIterator[interface{}]{}

type TreeIterator[T any] struct {
	*loser.Tree[T, Iterator[T]]
}

// NewTreeIterator returns an Iterator that iterates over the given loser tree iterator.
func NewTreeIterator[T any](tree *loser.Tree[T, Iterator[T]]) *TreeIterator[T] {
	return &TreeIterator[T]{
		Tree: tree,
	}
}

func (it TreeIterator[T]) At() T {
	return it.Tree.Winner().At()
}

func (it *TreeIterator[T]) Err() error {
	return it.Tree.Winner().Err()
}

func (it *TreeIterator[T]) Close() error {
	it.Tree.Close()
	return nil
}
