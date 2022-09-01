package parquetquery

import (
	"bytes"
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"

	"github.com/opentracing/opentracing-go"
	pq "github.com/segmentio/parquet-go"
)

// RowNumber is the sequence of row numbers uniquely identifying a value
// in a tree of nested columns, starting at the top-level and including
// another row number for each level of nesting. -1 is a placeholder
// for undefined at lower levels.  RowNumbers can be compared for full
// equality using the == operator, or can be compared partially, looking
// for equal lineages down to a certain level.
// For example given the following tree, the row numbers would be:
//
//	A          0, -1, -1
//	  B        0,  0, -1
//	  C        0,  1, -1
//	    D      0,  1,  0
//	  E        0,  2, -1
//
// Currently supports 6 levels of nesting which should be enough for anybody. :)
type RowNumber [6]int64

// EmptyRowNumber creates an empty invalid row number.
func EmptyRowNumber() RowNumber {
	return RowNumber{-1, -1, -1, -1, -1, -1}
}

// MaxRowNumber is a helper that represents the maximum(-ish) representable value.
func MaxRowNumber() RowNumber {
	return RowNumber{math.MaxInt64}
}

// CompareRowNumbers compares the sequences of row numbers in
// a and b for partial equality, descending from top-level
// through the given definition level.
// For example, definition level 1 means that row numbers are compared
// at two levels of nesting, the top-level and 1 level of nesting
// below.
func CompareRowNumbers(upToDefinitionLevel int, a, b RowNumber) int {
	for i := 0; i <= upToDefinitionLevel; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func TruncateRowNumber(definitionLevelToKeep int, t RowNumber) RowNumber {
	n := EmptyRowNumber()
	for i := 0; i <= definitionLevelToKeep; i++ {
		n[i] = t[i]
	}
	return n
}

func (t RowNumber) Valid() bool {
	return t[0] >= 0
}

// Next increments and resets the row numbers according
// to the given repetition and definition levels. Examples
// from the Dremel whitepaper:
// https://storage.googleapis.com/pub-tools-public-publication-data/pdf/36632.pdf
// Name.Language.Country
// value  | r | d | expected RowNumber
// -------|---|---|-------------------
//
//	|   |   | { -1, -1, -1, -1 }  <-- starting position
//
// us     | 0 | 3 | {  0,  0,  0,  0 }
// null   | 2 | 2 | {  0,  0,  1, -1 }
// null   | 1 | 1 | {  0,  1, -1, -1 }
// gb     | 1 | 3 | {  0,  2,  0,  0 }
// null   | 0 | 1 | {  1,  0, -1, -1 }
func (t *RowNumber) Next(repetitionLevel, definitionLevel int) {
	// Next row at this level
	t[repetitionLevel]++

	// New children up through the definition level
	for i := repetitionLevel + 1; i <= definitionLevel; i++ {
		t[i] = 0
	}

	// Children past the definition level are undefined
	for i := definitionLevel + 1; i < len(t); i++ {
		t[i] = -1
	}
}

// Skip rows at the root-level.
func (t *RowNumber) Skip(numRows int64) {
	t[0] += numRows
	for i := 1; i < len(t); i++ {
		t[i] = -1
	}
}

// IteratorResult is a row of data with a row number and named columns of data.
// Internally it has an unstructured list for efficient collection. The ToMap()
// function can be used to make inspection easier.
type IteratorResult struct {
	RowNumber RowNumber
	Entries   []struct {
		k string
		v pq.Value
	}
}

func (r *IteratorResult) Reset() {
	r.Entries = r.Entries[:0]
}

func (r *IteratorResult) Append(rr *IteratorResult) {
	r.Entries = append(r.Entries, rr.Entries...)
}

func (r *IteratorResult) AppendValue(k string, v pq.Value) {
	r.Entries = append(r.Entries, struct {
		k string
		v pq.Value
	}{k, v})
}

// ToMap converts the unstructured list of data into a map containing an entry
// for each column, and the lists of values.  The order of columns is
// not preseved, but the order of values within each column is.
func (r *IteratorResult) ToMap() map[string][]pq.Value {
	m := map[string][]pq.Value{}
	for _, e := range r.Entries {
		m[e.k] = append(m[e.k], e.v)
	}
	return m
}

// Columns gets the values for each named column. The order of returned values
// matches the order of names given. This is more efficient than converting to a map.
func (r *IteratorResult) Columns(buffer [][]pq.Value, names ...string) [][]pq.Value {
	if cap(buffer) < len(names) {
		buffer = make([][]pq.Value, len(names))
	}
	buffer = buffer[:len(names)]
	for i := range buffer {
		buffer[i] = buffer[i][:0]
	}

	for _, e := range r.Entries {
		for i := range names {
			if e.k == names[i] {
				buffer[i] = append(buffer[i], e.v)
				break
			}

		}
	}
	return buffer
}

// iterator - Every iterator follows this interface and can be composed.
type Iterator interface {
	// Next returns nil when done
	Next() *IteratorResult

	// Like Next but skips over results until reading >= the given location
	SeekTo(t RowNumber, definitionLevel int) *IteratorResult

	Close()
}

var columnIteratorPool = sync.Pool{
	New: func() interface{} {
		return &columnIteratorBuffer{}
	},
}

func columnIteratorPoolGet(capacity, len int) *columnIteratorBuffer {
	res := columnIteratorPool.Get().(*columnIteratorBuffer)
	if cap(res.rowNumbers) < capacity {
		res.rowNumbers = make([]RowNumber, capacity)
	}
	if cap(res.values) < capacity {
		res.values = make([]pq.Value, capacity)
	}
	res.rowNumbers = res.rowNumbers[:len]
	res.values = res.values[:len]
	return res
}

func columnIteratorPoolPut(b *columnIteratorBuffer) {
	b.values = b.values[:cap(b.values)]
	for i := range b.values {
		b.values[i] = pq.Value{}
	}
	columnIteratorPool.Put(b)
}

var columnIteratorResultPool = sync.Pool{
	New: func() interface{} {
		return &IteratorResult{Entries: make([]struct {
			k string
			v pq.Value
		}, 0, 10)} // For luck
	},
}

func columnIteratorResultPoolGet() *IteratorResult {
	res := columnIteratorResultPool.Get().(*IteratorResult)
	return res
}

func columnIteratorResultPoolPut(r *IteratorResult) {
	if r != nil {
		r.Reset()
		columnIteratorResultPool.Put(r)
	}
}

// ColumnIterator asynchronously iterates through the given row groups and column. Applies
// the optional predicate to each chunk, page, and value.  Results are read by calling
// Next() until it returns nil.
type ColumnIterator struct {
	rgs     []pq.RowGroup
	col     int
	colName string
	filter  *InstrumentedPredicate

	selectAs string
	seekTo   atomic.Value

	quit chan struct{}
	ch   chan *columnIteratorBuffer

	curr  *columnIteratorBuffer
	currN int
}

var _ Iterator = (*ColumnIterator)(nil)

type columnIteratorBuffer struct {
	rowNumbers []RowNumber
	values     []pq.Value
}

func NewColumnIterator(ctx context.Context, rgs []pq.RowGroup, column int, columnName string, readSize int, filter Predicate, selectAs string) *ColumnIterator {
	c := &ColumnIterator{
		rgs:      rgs,
		col:      column,
		colName:  columnName,
		filter:   &InstrumentedPredicate{pred: filter},
		selectAs: selectAs,
		quit:     make(chan struct{}),
		ch:       make(chan *columnIteratorBuffer, 1),
		currN:    -1,
	}

	go c.iterate(ctx, readSize)
	return c
}

func (c *ColumnIterator) iterate(ctx context.Context, readSize int) {
	defer close(c.ch)

	span, ctx2 := opentracing.StartSpanFromContext(ctx, "columnIterator.iterate", opentracing.Tags{
		"columnIndex": c.col,
		"column":      c.colName,
	})
	defer func() {
		span.SetTag("inspectedColumnChunks", c.filter.InspectedColumnChunks.Load())
		span.SetTag("inspectedPages", c.filter.InspectedPages.Load())
		span.SetTag("inspectedValues", c.filter.InspectedValues.Load())
		span.SetTag("keptColumnChunks", c.filter.KeptColumnChunks.Load())
		span.SetTag("keptPages", c.filter.KeptPages.Load())
		span.SetTag("keptValues", c.filter.KeptValues.Load())
		span.Finish()
	}()

	rn := EmptyRowNumber()
	buffer := make([]pq.Value, readSize)

	checkSkip := func(numRows int64) bool {
		seekTo := c.seekTo.Load()
		if seekTo == nil {
			return false
		}

		seekToRN := seekTo.(RowNumber)

		rnNext := rn
		rnNext.Skip(numRows)

		return CompareRowNumbers(0, rnNext, seekToRN) == -1
	}

	for _, rg := range c.rgs {
		col := rg.ColumnChunks()[c.col]

		if checkSkip(rg.NumRows()) {
			// Skip column chunk
			rn.Skip(rg.NumRows())
			continue
		}

		if c.filter != nil {
			if !c.filter.KeepColumnChunk(col) {
				// Skip column chunk
				rn.Skip(rg.NumRows())
				continue
			}
		}

		pgs := col.Pages()
		for {
			span2, _ := opentracing.StartSpanFromContext(ctx2, "columnIterator.iterate.ReadPage")
			pg, err := pgs.ReadPage()
			span2.Finish()

			if pg == nil || err == io.EOF {
				break
			}
			if err != nil {
				return
			}

			if checkSkip(pg.NumRows()) {
				// Skip page
				rn.Skip(pg.NumRows())
				continue
			}

			if c.filter != nil {
				if !c.filter.KeepPage(pg) {
					// Skip page
					rn.Skip(pg.NumRows())
					continue
				}
			}

			vr := pg.Values()
			for {
				count, err := vr.ReadValues(buffer)
				if count > 0 {

					// Assign row numbers, filter values, and collect the results.
					newBuffer := columnIteratorPoolGet(readSize, 0)

					for i := 0; i < count; i++ {

						v := buffer[i]

						// We have to do this for all values (even if the
						// value is excluded by the predicate)
						rn.Next(v.RepetitionLevel(), v.DefinitionLevel())

						if c.filter != nil {
							if !c.filter.KeepValue(v) {
								continue
							}
						}

						newBuffer.rowNumbers = append(newBuffer.rowNumbers, rn)
						newBuffer.values = append(newBuffer.values, v)
					}

					if len(newBuffer.rowNumbers) > 0 {
						select {
						case c.ch <- newBuffer:
						case <-c.quit:
							return
						}
					} else {
						// All values excluded, we go ahead and immediately
						// return the buffer to the pool.
						columnIteratorPoolPut(newBuffer)
					}
				}

				// Error checks MUST occur after processing any returned data
				// following io.Reader behavior.
				if err == io.EOF {
					break
				}
				if err != nil {
					// todo: bubble up?
					return
				}
			}
		}
	}
}

// Next returns the next matching value from the iterator.
// Returns nil when finished.
func (c *ColumnIterator) Next() *IteratorResult {

	t, v := c.next()
	if t.Valid() {
		return c.makeResult(t, v)
	}

	return nil
}

func (c *ColumnIterator) next() (RowNumber, pq.Value) {
	// Consume current buffer until exhausted
	// then read another one from the channel.
	if c.curr != nil {
		for c.currN++; c.currN < len(c.curr.rowNumbers); {
			t := c.curr.rowNumbers[c.currN]
			if t.Valid() {
				return t, c.curr.values[c.currN]
			}
		}

		// Done with this buffer
		columnIteratorPoolPut(c.curr)
		c.curr = nil
	}

	if v, ok := <-c.ch; ok {
		// Got next buffer, guaranteed to have at least 1 element
		c.curr = v
		c.currN = 0
		return c.curr.rowNumbers[0], c.curr.values[0]
	}

	// Failed to read from the channel, means iterator is exhausted.
	return EmptyRowNumber(), pq.Value{}
}

// SeekTo moves this iterator to the next result that is greater than
// or equal to the given row number (and based on the given definition level)
func (c *ColumnIterator) SeekTo(to RowNumber, d int) *IteratorResult {
	var at RowNumber
	var v pq.Value

	// Because iteration happens in the background, we signal the row
	// to skip to, and then read until we are at the right spot. The
	// seek is best-effort and may have no effect if the iteration
	// already further ahead, and there may already be older data
	// in the buffer.
	c.seekTo.Store(to)
	for at, v = c.next(); at.Valid() && CompareRowNumbers(d, at, to) < 0; {
		at, v = c.next()
	}

	if at.Valid() {
		return c.makeResult(at, v)
	}

	return nil
}

func (c *ColumnIterator) makeResult(t RowNumber, v pq.Value) *IteratorResult {
	r := columnIteratorResultPoolGet()
	r.RowNumber = t
	if c.selectAs != "" {
		r.AppendValue(c.selectAs, v)
	}
	return r
}

func (c *ColumnIterator) Close() {
	close(c.quit)
}

// JoinIterator joins two or more iterators for matches at the given definition level.
// I.e. joining at definitionLevel=0 means that each iterator must produce a result
// within the same root node.
type JoinIterator struct {
	definitionLevel int
	iters           []Iterator
	peeks           []*IteratorResult
	pred            GroupPredicate
}

var _ Iterator = (*JoinIterator)(nil)

func NewJoinIterator(definitionLevel int, iters []Iterator, pred GroupPredicate) *JoinIterator {
	j := JoinIterator{
		definitionLevel: definitionLevel,
		iters:           iters,
		peeks:           make([]*IteratorResult, len(iters)),
		pred:            pred,
	}
	return &j
}

func (j *JoinIterator) Next() *IteratorResult {

	// Here is the algorithm for joins:  On each pass of the iterators
	// we remember which ones are pointing at the earliest rows. If all
	// are the lowest (and therefore pointing at the same thing) then
	// there is a successful join and return the result.
	// Else we progress the iterators and try again.
	// There is an optimization here in that we can seek to the highest
	// row seen. It's impossible to have joins before that row.
	for {
		lowestRowNumber := MaxRowNumber()
		highestRowNumber := EmptyRowNumber()
		lowestIters := make([]int, 0, len(j.iters))

		for iterNum := range j.iters {
			res := j.peek(iterNum)

			if res == nil {
				// Iterator exhausted, no more joins possible
				return nil
			}

			c := CompareRowNumbers(j.definitionLevel, res.RowNumber, lowestRowNumber)
			switch c {
			case -1:
				// New lowest, reset
				lowestIters = lowestIters[:0]
				lowestRowNumber = res.RowNumber
				fallthrough

			case 0:
				// Same, append
				lowestIters = append(lowestIters, iterNum)
			}

			if CompareRowNumbers(j.definitionLevel, res.RowNumber, highestRowNumber) == 1 {
				// New high water mark
				highestRowNumber = res.RowNumber
			}
		}

		// All iterators pointing at same row?
		if len(lowestIters) == len(j.iters) {
			// Get the data
			result := j.collect(lowestRowNumber)

			// Keep group?
			if j.pred == nil || j.pred.KeepGroup(result) {
				// Yes
				return result
			}
		}

		// Skip all iterators to the highest row seen, it's impossible
		// to find matches before that.
		j.seekAll(highestRowNumber, j.definitionLevel)
	}
}

func (j *JoinIterator) SeekTo(t RowNumber, d int) *IteratorResult {
	j.seekAll(t, d)
	return j.Next()
}

func (j *JoinIterator) seekAll(t RowNumber, d int) {
	t = TruncateRowNumber(d, t)
	for iterNum, iter := range j.iters {
		if j.peeks[iterNum] == nil || CompareRowNumbers(d, j.peeks[iterNum].RowNumber, t) == -1 {
			columnIteratorResultPoolPut(j.peeks[iterNum])
			j.peeks[iterNum] = iter.SeekTo(t, d)
		}
	}
}

func (j *JoinIterator) peek(iterNum int) *IteratorResult {
	if j.peeks[iterNum] == nil {
		j.peeks[iterNum] = j.iters[iterNum].Next()
	}
	return j.peeks[iterNum]
}

// Collect data from the given iterators until they point at
// the next row (according to the configured definition level)
// or are exhausted.
func (j *JoinIterator) collect(rowNumber RowNumber) *IteratorResult {
	result := columnIteratorResultPoolGet()
	result.RowNumber = rowNumber

	for i := range j.iters {
		for j.peeks[i] != nil && CompareRowNumbers(j.definitionLevel, j.peeks[i].RowNumber, rowNumber) == 0 {

			result.Append(j.peeks[i])

			columnIteratorResultPoolPut(j.peeks[i])

			j.peeks[i] = j.iters[i].Next()
		}
	}
	return result
}

func (j *JoinIterator) Close() {
	for _, i := range j.iters {
		i.Close()
	}
}

// UnionIterator produces all results for all given iterators.  When iterators
// align to the same row, based on the configured definition level, then the results
// are returned together. Else the next matching iterator is returned.
type UnionIterator struct {
	definitionLevel int
	iters           []Iterator
	peeks           []*IteratorResult
	pred            GroupPredicate
}

var _ Iterator = (*UnionIterator)(nil)

func NewUnionIterator(definitionLevel int, iters []Iterator, pred GroupPredicate) *UnionIterator {
	j := UnionIterator{
		definitionLevel: definitionLevel,
		iters:           iters,
		peeks:           make([]*IteratorResult, len(iters)),
		pred:            pred,
	}
	return &j
}

func (u *UnionIterator) Next() *IteratorResult {

	// Here is the algorithm for unions:  On each pass of the iterators
	// we remember which ones are pointing at the earliest same row. The
	// lowest iterators are then collected and a result is produced. Keep
	// going until all iterators are exhausted.
	for {
		lowestRowNumber := MaxRowNumber()
		lowestIters := make([]int, 0, len(u.iters))

		for iterNum := range u.iters {
			rn := u.peek(iterNum)

			// If this iterator is exhausted go to the next one
			if rn == nil {
				continue
			}

			c := CompareRowNumbers(u.definitionLevel, rn.RowNumber, lowestRowNumber)
			switch c {
			case -1:
				// New lowest
				lowestIters = lowestIters[:0]
				lowestRowNumber = rn.RowNumber
				fallthrough

			case 0:
				// Same
				lowestIters = append(lowestIters, iterNum)
			}
		}

		// Consume lowest iterators
		result := u.collect(lowestIters, lowestRowNumber)

		// After each pass it is guaranteed to have found something
		// from at least one iterator, or all are exhausted
		if len(lowestIters) > 0 {
			if u.pred != nil && !u.pred.KeepGroup(result) {
				continue
			}

			return result
		}

		// All exhausted
		return nil
	}
}

func (u *UnionIterator) SeekTo(t RowNumber, d int) *IteratorResult {
	t = TruncateRowNumber(d, t)
	for iterNum, iter := range u.iters {
		if p := u.peeks[iterNum]; p == nil || CompareRowNumbers(d, p.RowNumber, t) == -1 {
			u.peeks[iterNum] = iter.SeekTo(t, d)
		}
	}
	return u.Next()
}

func (u *UnionIterator) peek(iterNum int) *IteratorResult {
	if u.peeks[iterNum] == nil {
		u.peeks[iterNum] = u.iters[iterNum].Next()
	}
	return u.peeks[iterNum]
}

// Collect data from the given iterators until they point at
// the next row (according to the configured definition level)
// or are exhausted.
func (u *UnionIterator) collect(iterNums []int, rowNumber RowNumber) *IteratorResult {
	result := columnIteratorResultPoolGet()
	result.RowNumber = rowNumber

	for _, iterNum := range iterNums {
		for u.peeks[iterNum] != nil && CompareRowNumbers(u.definitionLevel, u.peeks[iterNum].RowNumber, rowNumber) == 0 {

			result.Append(u.peeks[iterNum])

			columnIteratorResultPoolPut(u.peeks[iterNum])

			u.peeks[iterNum] = u.iters[iterNum].Next()
		}
	}

	return result
}

func (u *UnionIterator) Close() {
	for _, i := range u.iters {
		i.Close()
	}
}

type GroupPredicate interface {
	KeepGroup(*IteratorResult) bool
}

// KeyValueGroupPredicate takes key/value pairs and checks if the
// group contains all of them. This is the only predicate/iterator
// that is knowledgable about our trace or search contents. I'd like
// to change that and make it generic, but it's quite complex and not
// figured it out yet.
type KeyValueGroupPredicate struct {
	keys   [][]byte
	vals   [][]byte
	buffer [][]pq.Value
}

var _ GroupPredicate = (*KeyValueGroupPredicate)(nil)

func NewKeyValueGroupPredicate(keys, values []string) *KeyValueGroupPredicate {
	// Pre-convert all to bytes
	p := &KeyValueGroupPredicate{}
	for _, k := range keys {
		p.keys = append(p.keys, []byte(k))
	}
	for _, v := range values {
		p.vals = append(p.vals, []byte(v))
	}
	return p
}

// KeepGroup checks if the given group contains all of the requested
// key/value pairs.
func (a *KeyValueGroupPredicate) KeepGroup(group *IteratorResult) bool {
	//printGroup(group)
	a.buffer = group.Columns(a.buffer, "keys", "values")

	keys, vals := a.buffer[0], a.buffer[1]

	if len(keys) < len(a.keys) || len(keys) != len(vals) {
		// Missing data or unsatisfiable condition
		return false
	}

	for i := 0; i < len(a.keys); i++ {
		k := a.keys[i]
		v := a.vals[i]

		// Make sure k and v exist somewhere
		found := false

		for j := 0; j < len(keys) && j < len(vals); j++ {
			if bytes.Equal(k, keys[j].ByteArray()) && bytes.Equal(v, vals[j].ByteArray()) {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}
	return true
}

/*func printGroup(g *iteratorResult) {
	fmt.Println("---group---")
	for _, e := range g.entries {
		fmt.Println("key:", e.k)
		fmt.Println(" : ", e.v.String())
	}
}*/
