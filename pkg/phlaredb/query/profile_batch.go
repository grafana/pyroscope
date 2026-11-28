package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/parquet-go/parquet-go"
)

// ErrUnsupportedProfileBatchSchema permits callers to fall back before any I/O.
var ErrUnsupportedProfileBatchSchema = errors.New("unsupported profile batch schema")

// ProfileBatch contains selected rows in physical order. Slices are borrowed
// until the next Next or Close. Partitions is nil when not requested.
type ProfileBatch struct {
	RowNumbers    []int64
	SeriesIndexes []uint32
	TimeNanos     []int64
	Partitions    []uint64
}

// ProfileBatchScanner joins required flat columns in bounded batches. It reuses
// SyncIterator's chunk/page pruning and forward seeking, but avoids per-value
// result objects and named lookups, and amortizes row tracking and instrumentation.
// It owns its readers, but not the row groups or the immutable matches map.
type ProfileBatchScanner struct {
	ctx                       context.Context
	series, times, partitions *flatProfileCursor
	batch                     ProfileBatch
	size                      int
	err                       error
	closed                    bool
}

// NewProfileBatchScanner validates all schemas before opening readers. Unsupported
// schemas return ErrUnsupportedProfileBatchSchema so callers can choose a scalar plan.
func NewProfileBatchScanner(ctx context.Context, groups []parquet.RowGroup, seriesColumn, timeColumn, size int, matches map[uint32]struct{}, start, end int64, partitionColumn ...int) (*ProfileBatchScanner, error) {
	if size <= 0 {
		return nil, fmt.Errorf("profile batch size must be positive")
	}
	if len(partitionColumn) > 1 {
		return nil, fmt.Errorf("at most one partition column is allowed")
	}
	cols := append([]int{seriesColumn, timeColumn}, partitionColumn...)
	for i, col := range cols {
		for _, other := range cols[:i] {
			if col == other {
				return nil, fmt.Errorf("%w: duplicate column", ErrUnsupportedProfileBatchSchema)
			}
		}
		for _, rg := range groups {
			paths := rg.Schema().Columns()
			if col < 0 || col >= len(paths) {
				return nil, fmt.Errorf("%w: column %d", ErrUnsupportedProfileBatchSchema, col)
			}
			leaf, ok := rg.Schema().Lookup(paths[col]...)
			kind := parquet.Int64
			if i == 0 {
				kind = parquet.Int32
			}
			if !ok || leaf.MaxDefinitionLevel != 0 || leaf.MaxRepetitionLevel != 0 || leaf.Node.Type().Kind() != kind {
				return nil, fmt.Errorf("%w: column %d must be a required flat %s", ErrUnsupportedProfileBatchSchema, col, kind)
			}
			if i == 0 {
				logical := leaf.Node.Type().LogicalType()
				if logical == nil || logical.Integer == nil || logical.Integer.IsSigned || logical.Integer.BitWidth != 32 {
					return nil, fmt.Errorf("%w: series index must be uint32", ErrUnsupportedProfileBatchSchema)
				}
			}
		}
	}
	s := &ProfileBatchScanner{ctx: ctx, size: size}
	if len(groups) == 0 || len(matches) == 0 || start > end {
		return s, nil
	}
	makeCursor := func(col int, name string, p Predicate) *flatProfileCursor {
		return &flatProfileCursor{it: NewSyncIterator(ctx, groups, col, name, size, p, "")}
	}
	s.series = makeCursor(seriesColumn, "SeriesIndex", newProfileSeriesPredicate(matches))
	s.times = makeCursor(timeColumn, "TimeNanos", &profileTimePredicate{NewIntBetweenPredicate(start, end)})
	if len(partitionColumn) > 0 {
		s.partitions = makeCursor(partitionColumn[0], "StacktracePartition", nil)
	}
	return s, nil
}

func (s *ProfileBatchScanner) At() ProfileBatch { return s.batch }
func (s *ProfileBatchScanner) Err() error       { return s.err }
func (s *ProfileBatchScanner) Next() bool {
	s.batch.RowNumbers = s.batch.RowNumbers[:0]
	s.batch.SeriesIndexes = s.batch.SeriesIndexes[:0]
	s.batch.TimeNanos = s.batch.TimeNanos[:0]
	if s.batch.Partitions != nil {
		s.batch.Partitions = s.batch.Partitions[:0]
	}
	if s.closed || s.err != nil {
		return false
	}
	if s.err = s.ctx.Err(); s.err != nil {
		return false
	}
	if s.series == nil {
		return false
	}
	for len(s.batch.RowNumbers) < s.size {
		if !s.series.ensure(0) {
			s.err = s.series.err
			break
		}
		row := s.series.rows[s.series.pos]
		if !s.times.ensure(row) {
			s.err = s.times.err
			break
		}
		if other := s.times.rows[s.times.pos]; other > row {
			if !s.series.ensure(other) {
				s.err = s.series.err
				break
			}
			continue
		}
		if s.batch.RowNumbers == nil {
			s.batch.RowNumbers = make([]int64, 0, s.size)
			s.batch.SeriesIndexes = make([]uint32, 0, s.size)
			s.batch.TimeNanos = make([]int64, 0, s.size)
			if s.partitions != nil {
				s.batch.Partitions = make([]uint64, 0, s.size)
			}
		}
		if s.partitions != nil {
			if !s.partitions.ensure(row) {
				s.err = s.partitions.err
				if s.err == nil {
					s.err = fmt.Errorf("missing partition for profile row %d", row)
				}
				break
			}
			if s.partitions.rows[s.partitions.pos] != row {
				s.err = fmt.Errorf("missing partition for profile row %d", row)
				break
			}
			s.batch.Partitions = append(s.batch.Partitions, s.partitions.values[s.partitions.pos])
		}
		s.batch.RowNumbers = append(s.batch.RowNumbers, row)
		s.batch.SeriesIndexes = append(s.batch.SeriesIndexes, uint32(s.series.values[s.series.pos]))
		s.batch.TimeNanos = append(s.batch.TimeNanos, int64(s.times.values[s.times.pos]))
		s.series.pos++
		s.times.pos++
	}
	return s.err == nil && len(s.batch.RowNumbers) > 0
}
func (s *ProfileBatchScanner) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	for _, c := range []*flatProfileCursor{s.series, s.times, s.partitions} {
		if c != nil {
			err = errors.Join(err, c.it.Close())
			c.rows = nil
			c.values = nil
		}
	}
	s.batch = ProfileBatch{}
	return err
}

type flatProfileCursor struct {
	it     *SyncIterator
	rows   []int64
	values []uint64
	pos    int
	err    error
	done   bool
}

// ensure positions at the first matching row >= target. Only exhausted batches
// refill; distant targets reuse the existing iterator's page-skipping policy.
func (c *flatProfileCursor) ensure(target int64) bool {
	for {
		for c.pos < len(c.rows) {
			if c.rows[c.pos] >= target {
				return true
			}
			c.pos++
		}
		if c.done || c.err != nil {
			return false
		}
		c.rows = c.rows[:0]
		c.values = c.values[:0]
		c.pos = 0
		to := EmptyRowNumber()
		to[0] = target
		done, err := c.it.seekRowGroup(to, 0)
		if err != nil || done {
			c.err = err
			c.done = done
			return false
		}
		// seekPages can exhaust one row group; next() handles advancing to the next.
		_, err = c.it.seekPages(to, 0)
		if err != nil {
			c.err = err
			return false
		}
		rn, v, err := c.it.next()
		if err != nil {
			c.err = err
			return false
		}
		if !rn.Valid() {
			c.done = true
			return false
		}
		if c.rows == nil {
			c.rows = make([]int64, 0, c.it.readSize)
			c.values = make([]uint64, 0, c.it.readSize)
		}
		if rn[0] >= target {
			c.rows = append(c.rows, rn[0])
			c.values = append(c.values, v.Uint64())
		}
		// next() has loaded and filtered the first value. Consume the rest of its
		// decode buffer directly, updating instrumentation once per buffer.
		var inspected, kept int64
		for c.it.currBufN < len(c.it.currBuf) {
			value := c.it.currBuf[c.it.currBufN]
			c.it.currBufN++
			c.it.curr[0]++
			inspected++
			if c.it.filter.pred != nil && !c.it.filter.pred.KeepValue(value) {
				continue
			}
			kept++
			if c.it.curr[0] >= target {
				c.rows = append(c.rows, c.it.curr[0])
				c.values = append(c.values, value.Uint64())
			}
		}
		c.it.filter.InspectedValues.Add(inspected)
		c.it.filter.KeptValues.Add(kept)
	}
}

// Missing or unusable bounds must never exclude a candidate page.
type profileTimePredicate struct{ *IntBetweenPredicate }

func (p *profileTimePredicate) bounds(min, max parquet.Value) bool {
	if min.IsNull() || max.IsNull() {
		return true
	}
	lo, hi := min.Int64(), max.Int64()
	return lo > hi || (p.max >= lo && p.min <= hi)
}

func (p *profileTimePredicate) KeepColumnChunk(ci parquet.ColumnIndex) bool {
	if ci == nil || ci.NumPages() == 0 {
		return true
	}
	for i := 0; i < ci.NumPages(); i++ {
		if p.bounds(ci.MinValue(i), ci.MaxValue(i)) {
			return true
		}
	}
	return false
}

func (p *profileTimePredicate) KeepPage(page parquet.Page) bool {
	min, max, ok := page.Bounds()
	return !ok || p.bounds(min, max)
}

// SeriesIndex is physically INT32 but logically unsigned. Signed bounds would
// incorrectly prune pages containing series indexes above MaxInt32.
type profileSeriesPredicate struct {
	matches  map[uint32]struct{}
	min, max uint32
}

func newProfileSeriesPredicate(matches map[uint32]struct{}) *profileSeriesPredicate {
	p := &profileSeriesPredicate{matches: matches, min: ^uint32(0)}
	for v := range matches {
		if v < p.min {
			p.min = v
		}
		if v > p.max {
			p.max = v
		}
	}
	return p
}
func (p *profileSeriesPredicate) KeepValue(v parquet.Value) bool {
	_, ok := p.matches[v.Uint32()]
	return ok
}
func (p *profileSeriesPredicate) bounds(min, max parquet.Value) bool {
	if min.IsNull() || max.IsNull() {
		return true
	}
	lo, hi := min.Uint32(), max.Uint32()
	return lo > hi || (p.max >= lo && p.min <= hi)
}
func (p *profileSeriesPredicate) KeepColumnChunk(ci parquet.ColumnIndex) bool {
	if ci == nil || ci.NumPages() == 0 {
		return true
	}
	for i := 0; i < ci.NumPages(); i++ {
		if p.bounds(ci.MinValue(i), ci.MaxValue(i)) {
			return true
		}
	}
	return false
}
func (p *profileSeriesPredicate) KeepPage(page parquet.Page) bool {
	min, max, ok := page.Bounds()
	return !ok || p.bounds(min, max)
}
