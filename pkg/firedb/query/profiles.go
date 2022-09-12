package query

import (
	"container/heap"

	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/dskit/multierror"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
)

type Profile struct {
	Labels      firemodel.Labels
	Fingerprint model.Fingerprint
	Timestamp   model.Time
	RowNum      int64
}

func (p Profile) RowNumber() int64 {
	return p.RowNum
}

type ProfileValue struct {
	Profile
	Value int64
}

type SeriesIterator struct {
	rowNums Iterator

	curr   Profile
	buffer [][]parquet.Value
}

func NewSeriesIterator(rowNums Iterator, fp model.Fingerprint, lbs firemodel.Labels) *SeriesIterator {
	return &SeriesIterator{
		rowNums: rowNums,
		curr:    Profile{Fingerprint: fp, Labels: lbs},
	}
}

func (p *SeriesIterator) Next() bool {
	if !p.rowNums.Next() {
		return false
	}
	if p.buffer == nil {
		p.buffer = make([][]parquet.Value, 2)
	}
	result := p.rowNums.At()
	p.curr.RowNum = result.RowNumber[0]
	p.buffer = result.Columns(p.buffer, "TimeNanos")
	p.curr.Timestamp = model.TimeFromUnixNano(p.buffer[0][0].Int64())
	return true
}

func (p *SeriesIterator) At() Profile {
	return p.curr
}

func (p *SeriesIterator) Err() error {
	return p.rowNums.Err()
}

func (p *SeriesIterator) Close() error {
	return p.rowNums.Close()
}

type ProfileIteratorHeap []iter.Iterator[Profile]

func (h ProfileIteratorHeap) Len() int { return len(h) }
func (h ProfileIteratorHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *ProfileIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(iter.Iterator[Profile]))
}

func (h *ProfileIteratorHeap) Pop() interface{} {
	n := len(*h)
	x := (*h)[n-1]
	*h = (*h)[0 : n-1]
	return x
}

func (h ProfileIteratorHeap) Less(i, j int) bool {
	p1, p2 := h[i].At(), h[j].At()
	if p1.Timestamp == p2.Timestamp {
		// todo we could compare SeriesRef here
		return firemodel.CompareLabelPairs(p1.Labels, p2.Labels) < 0
	}
	return p1.Timestamp < p2.Timestamp
}

type SortIterator struct {
	heap *ProfileIteratorHeap
	errs []error
	curr Profile
}

// NewSortIterator sorts the input iterator by timestamp then labels.
// Each input iterator must return Profile in ascending time.
func NewSortIterator(iters []iter.Iterator[Profile]) iter.Iterator[Profile] {
	h := make(ProfileIteratorHeap, 0, len(iters))
	res := &SortIterator{
		heap: &h,
	}
	for _, iter := range iters {
		res.requeue(iter)
	}
	return res
}

func (i *SortIterator) requeue(ei iter.Iterator[Profile]) {
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

func (s *SortIterator) Next() bool {
	if s.heap.Len() == 0 {
		return false
	}
	next := heap.Pop(s.heap).(iter.Iterator[Profile])
	s.curr = next.At()
	s.requeue(next)
	return true
}

func (s *SortIterator) At() Profile {
	return s.curr
}

func (i *SortIterator) Err() error {
	return multierror.New(i.errs...).Err()
}

func (i *SortIterator) Close() error {
	for _, s := range *i.heap {
		s.Close()
		if err := s.Err(); err != nil {
			i.errs = append(i.errs, err)
		}
	}
	return i.Err()
}
