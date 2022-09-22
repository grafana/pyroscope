package iter

import (
	"container/heap"

	"github.com/grafana/dskit/multierror"
	"github.com/prometheus/common/model"

	firemodel "github.com/grafana/fire/pkg/model"
)

type Timestamp interface {
	Timestamp() model.Time
}

type Profile interface {
	Labels() firemodel.Labels
	Timestamp
}

type ProfileIteratorHeap[P Profile] []Iterator[P]

func (h ProfileIteratorHeap[P]) Len() int { return len(h) }
func (h ProfileIteratorHeap[P]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *ProfileIteratorHeap[P]) Push(x interface{}) {
	*h = append(*h, x.(Iterator[P]))
}

func (h *ProfileIteratorHeap[P]) Pop() interface{} {
	n := len(*h)
	x := (*h)[n-1]
	*h = (*h)[0 : n-1]
	return x
}

func (h ProfileIteratorHeap[P]) Less(i, j int) bool {
	p1, p2 := h[i].At(), h[j].At()
	if p1.Timestamp() == p2.Timestamp() {
		// todo we could compare SeriesRef here
		return firemodel.CompareLabelPairs(p1.Labels(), p2.Labels()) < 0
	}
	return p1.Timestamp() < p2.Timestamp()
}

type SortIterator[P Profile] struct {
	heap *ProfileIteratorHeap[P]
	errs []error
	curr P
}

// NewSortProfileIterator sorts the input iterator by timestamp then labels.
// Each input iterator must return Profile in ascending time.
func NewSortProfileIterator[P Profile](iters []Iterator[P]) Iterator[P] {
	h := make(ProfileIteratorHeap[P], 0, len(iters))
	res := &SortIterator[P]{
		heap: &h,
	}
	for _, iter := range iters {
		res.requeue(iter)
	}
	return res
}

func (i *SortIterator[P]) requeue(ei Iterator[P]) {
	if ei.Next() {
		heap.Push(i.heap, ei)
		return
	}
	if err := ei.Err(); err != nil {
		i.errs = append(i.errs, err)
	}
	if err := ei.Close(); err != nil {
		i.errs = append(i.errs, err)
	}
}

func (s *SortIterator[P]) Next() bool {
	if s.heap.Len() == 0 {
		return false
	}
	next := heap.Pop(s.heap).(Iterator[P])
	s.curr = next.At()
	s.requeue(next)
	return true
}

func (s *SortIterator[P]) At() P {
	return s.curr
}

func (i *SortIterator[P]) Err() error {
	return multierror.New(i.errs...).Err()
}

func (i *SortIterator[P]) Close() error {
	for _, s := range *i.heap {
		s.Close()
		if err := s.Err(); err != nil {
			i.errs = append(i.errs, err)
		}
	}
	return i.Err()
}

type TimeRangedIterator[T Timestamp] struct {
	Iterator[T]
	min, max model.Time
}

func NewTimeRangedIterator[T Timestamp](it Iterator[T], min, max model.Time) Iterator[T] {
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
