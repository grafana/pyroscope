package model

import (
	"github.com/grafana/dskit/multierror"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type Timestamp interface {
	Timestamp() model.Time
}

type Profile interface {
	Labels() Labels
	Timestamp
}

func lessProfile(p1, p2 Profile) bool {
	if p1.Timestamp() == p2.Timestamp() {
		// todo we could compare SeriesRef here
		return CompareLabelPairs(p1.Labels(), p2.Labels()) < 0
	}
	return p1.Timestamp() < p2.Timestamp()
}

type MergeIterator[P Profile] struct {
	tree        *loser.Tree[P, iter.Iterator[P]]
	closeErrs   multierror.MultiError
	current     P
	deduplicate bool
}

// NewMergeIterator returns an iterator that k-way merges the given iterators.
// The given iterators must be sorted by timestamp and labels themselves.
// Optionally, the iterator can deduplicate profiles with the same timestamp and labels.
func NewMergeIterator[P Profile](max P, deduplicate bool, iters ...iter.Iterator[P]) iter.Iterator[P] {
	if len(iters) == 0 {
		return iter.NewEmptyIterator[P]()
	}
	if len(iters) == 1 {
		// No need to merge a single iterator.
		// We should never allow a single iterator to be passed in because
		return iters[0]
	}
	m := &MergeIterator[P]{
		deduplicate: deduplicate,
		current:     max,
	}
	m.tree = loser.New(
		iters,
		max,
		func(s iter.Iterator[P]) P {
			return s.At()
		},
		func(p1, p2 P) bool {
			return lessProfile(p1, p2)
		},
		func(s iter.Iterator[P]) {
			if err := s.Close(); err != nil {
				m.closeErrs.Add(err)
			}
		})
	return m
}

func (i *MergeIterator[P]) Next() bool {
	for i.tree.Next() {
		next := i.tree.Winner()

		if !i.deduplicate {
			i.current = next.At()
			return true
		}
		if next.At().Timestamp() != i.current.Timestamp() ||
			CompareLabelPairs(next.At().Labels(), i.current.Labels()) != 0 {
			i.current = next.At()
			return true
		}

	}
	return false
}

func (i *MergeIterator[P]) At() P {
	return i.current
}

func (i *MergeIterator[P]) Err() error {
	return i.tree.Err()
}

func (i *MergeIterator[P]) Close() error {
	i.tree.Close()
	return i.closeErrs.Err()
}

type TimeRangedIterator[T Timestamp] struct {
	iter.Iterator[T]
	min, max model.Time
}

func NewTimeRangedIterator[T Timestamp](it iter.Iterator[T], min, max model.Time) iter.Iterator[T] {
	return &TimeRangedIterator[T]{
		Iterator: it,
		min:      min,
		max:      max,
	}
}

func (it *TimeRangedIterator[T]) Next() bool {
	for it.Iterator.Next() {
		if it.At().Timestamp() < it.min {
			continue
		}
		if it.At().Timestamp() > it.max {
			return false
		}
		return true
	}
	return false
}
