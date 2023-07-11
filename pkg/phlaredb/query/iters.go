package query

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/phlare/pkg/iter"
)

const MaxDefinitionLevel = 5

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
type RowNumber [MaxDefinitionLevel + 1]int64

type RowNumberWithDefinitionLevel struct {
	RowNumber       RowNumber
	DefinitionLevel int
}

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

func TruncateRowNumber(t RowNumberWithDefinitionLevel) RowNumber {
	n := EmptyRowNumber()
	for i := 0; i <= t.DefinitionLevel; i++ {
		n[i] = t.RowNumber[i]
	}
	return n
}

func (t *RowNumber) Valid() bool {
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
	t[repetitionLevel]++

	// the following is nextSlow() unrolled
	switch repetitionLevel {
	case 0:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[1] = 0
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[1] = 0
			t[2] = 0
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[1] = 0
			t[2] = 0
			t[3] = 0
			t[4] = -1
			t[5] = -1
		case 4:
			t[1] = 0
			t[2] = 0
			t[3] = 0
			t[4] = 0
			t[5] = -1
		case 5:
			t[1] = 0
			t[2] = 0
			t[3] = 0
			t[4] = 0
			t[5] = 0
		}
	case 1:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[2] = 0
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[2] = 0
			t[3] = 0
			t[4] = -1
			t[5] = -1
		case 4:
			t[2] = 0
			t[3] = 0
			t[4] = 0
			t[5] = -1
		case 5:
			t[2] = 0
			t[3] = 0
			t[4] = 0
			t[5] = 0
		}
	case 2:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[3] = 0
			t[4] = -1
			t[5] = -1
		case 4:
			t[3] = 0
			t[4] = 0
			t[5] = -1
		case 5:
			t[3] = 0
			t[4] = 0
			t[5] = 0
		}
	case 3:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[4] = -1
			t[5] = -1
		case 4:
			t[4] = 0
			t[5] = -1
		case 5:
			t[4] = 0
			t[5] = 0
		}
	case 4:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[4] = -1
			t[5] = -1
		case 4:
			t[5] = -1
		case 5:
			t[5] = 0
		}
	case 5:
		switch definitionLevel {
		case 0:
			t[1] = -1
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 1:
			t[2] = -1
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 2:
			t[3] = -1
			t[4] = -1
			t[5] = -1
		case 3:
			t[4] = -1
			t[5] = -1
		case 4:
			t[5] = -1
		}
	}
}

// nextSlow is the original implementation of next. it is kept to test against
// the unrolled version above
func (t *RowNumber) nextSlow(repetitionLevel, definitionLevel int) {
	t[repetitionLevel]++

	// New children up through the definition level
	for i := repetitionLevel + 1; i <= definitionLevel; i++ {
		t[i] = 0
	}

	// // Children past the definition level are undefined
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

// Preceding returns the largest representable row number that is immediately prior to this
// one. Think of it like math.NextAfter but for segmented row numbers. Examples:
//
//		RowNumber 1000.0.0 (defined at 3 levels) is preceded by 999.max.max
//	    RowNumber 1000.-1.-1 (defined at 1 level) is preceded by 999.-1.-1
func (t RowNumber) Preceding() RowNumber {
	for i := len(t) - 1; i >= 0; i-- {
		switch t[i] {
		case -1:
			continue
		case 0:
			t[i] = math.MaxInt64
		default:
			t[i]--
			return t
		}
	}
	return t
}

// IteratorResult is a row of data with a row number and named columns of data.
// Internally it has an unstructured list for efficient collection. The ToMap()
// function can be used to make inspection easier.
type IteratorResult struct {
	RowNumber RowNumber
	Entries   []struct {
		k        string
		V        parquet.Value
		RowValue interface{}
	}
}

func (r *IteratorResult) Reset() {
	r.Entries = r.Entries[:0]
}

func (r *IteratorResult) Append(rr *IteratorResult) {
	r.Entries = append(r.Entries, rr.Entries...)
}

func (r *IteratorResult) AppendValue(k string, v parquet.Value) {
	r.Entries = append(r.Entries, struct {
		k        string
		V        parquet.Value
		RowValue interface{}
	}{k, v, nil})
}

// ToMap converts the unstructured list of data into a map containing an entry
// for each column, and the lists of values.  The order of columns is
// not preseved, but the order of values within each column is.
func (r *IteratorResult) ToMap() map[string][]parquet.Value {
	m := map[string][]parquet.Value{}
	for _, e := range r.Entries {
		m[e.k] = append(m[e.k], e.V)
	}
	return m
}

// Columns gets the values for each named column. The order of returned values
// matches the order of names given. This is more efficient than converting to a map.
func (r *IteratorResult) Columns(buffer [][]parquet.Value, names ...string) [][]parquet.Value {
	if cap(buffer) < len(names) {
		buffer = make([][]parquet.Value, len(names))
	}
	buffer = buffer[:len(names)]
	for i := range buffer {
		buffer[i] = buffer[i][:0]
	}

	for _, e := range r.Entries {
		for i := range names {
			if e.k == names[i] {
				buffer[i] = append(buffer[i], e.V)
				break
			}
		}
	}
	return buffer
}

// iterator - Every iterator follows this interface and can be composed.
type Iterator = iter.SeekIterator[*IteratorResult, RowNumberWithDefinitionLevel]

func NewErrIterator(err error) Iterator {
	return iter.NewErrSeekIterator[*IteratorResult, RowNumberWithDefinitionLevel](err)
}

var iteratorResultPool = sync.Pool{
	New: func() interface{} {
		return &IteratorResult{Entries: make([]struct {
			k        string
			V        parquet.Value
			RowValue interface{}
		}, 0, 10)} // For luck
	},
}

func iteratorResultPoolGet() *IteratorResult {
	res := iteratorResultPool.Get().(*IteratorResult)
	return res
}

func iteratorResultPoolPut(r *IteratorResult) {
	if r != nil {
		r.Reset()
		iteratorResultPool.Put(r)
	}
}

type BinaryJoinIterator struct {
	left            Iterator
	right           Iterator
	definitionLevel int

	err error
	res *IteratorResult
}

var _ Iterator = (*BinaryJoinIterator)(nil)

func NewBinaryJoinIterator(definitionLevel int, left, right Iterator) *BinaryJoinIterator {
	return &BinaryJoinIterator{
		left:            left,
		right:           right,
		definitionLevel: definitionLevel,
	}
}

// nextOrSeek will use next if the iterator is exactly one row aways
func (bj *BinaryJoinIterator) nextOrSeek(to RowNumberWithDefinitionLevel, it Iterator) bool {
	// Seek when definition level is higher then 0, there is not previous iteration or when the difference between current position and to is not 1
	if to.DefinitionLevel != 0 || it.At() == nil || to.RowNumber.Preceding() != it.At().RowNumber {
		return it.Seek(to)
	}
	return it.Next()
}

func (bj *BinaryJoinIterator) Next() bool {
	for {
		if !bj.left.Next() {
			bj.err = bj.left.Err()
			return false
		}
		resLeft := bj.left.At()

		// now seek the right iterator to the left position
		if !bj.nextOrSeek(RowNumberWithDefinitionLevel{resLeft.RowNumber, bj.definitionLevel}, bj.right) {
			bj.err = bj.right.Err()
			return false
		}
		resRight := bj.right.At()

		makeResult := func() {
			bj.res = iteratorResultPoolGet()
			bj.res.RowNumber = resLeft.RowNumber
			bj.res.Append(resLeft)
			bj.res.Append(resRight)
			iteratorResultPoolPut(resLeft)
			iteratorResultPoolPut(resRight)
		}

		if cmp := CompareRowNumbers(bj.definitionLevel, resLeft.RowNumber, resRight.RowNumber); cmp == 0 {
			// we have a found an element
			makeResult()
			return true
		} else if cmp < 0 {
			if !bj.nextOrSeek(RowNumberWithDefinitionLevel{resRight.RowNumber, bj.definitionLevel}, bj.left) {
				bj.err = bj.left.Err()
				return false
			}
			resLeft = bj.left.At()

			if cmp := CompareRowNumbers(bj.definitionLevel, resLeft.RowNumber, resRight.RowNumber); cmp == 0 {
				makeResult()
				return true
			}

		} else {
			panic("bug in iterator during join: the right iterator cannot be smaller than the left one, as it just has been Seeked beyond")
		}
	}
}

func (bj *BinaryJoinIterator) At() *IteratorResult {
	return bj.res
}

func (bj *BinaryJoinIterator) Seek(to RowNumberWithDefinitionLevel) bool {
	bj.left.Seek(to)
	bj.right.Seek(to)
	return bj.Next()
}

func (bj *BinaryJoinIterator) Close() error {
	var merr multierror.MultiError
	merr.Add(bj.left.Close())
	merr.Add(bj.right.Close())
	return merr.Err()
}

func (c *BinaryJoinIterator) Err() error {
	return c.err
}

// UnionIterator produces all results for all given iterators.  When iterators
// align to the same row, based on the configured definition level, then the results
// are returned together. Else the next matching iterator is returned.
type UnionIterator struct {
	definitionLevel int
	iters           []Iterator
	peeks           []*IteratorResult
	pred            GroupPredicate
	result          *IteratorResult
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

func (u *UnionIterator) At() *IteratorResult {
	return u.result
}

func (u *UnionIterator) Next() bool {
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

			u.result = result
			return true
		}

		// All exhausted
		u.result = nil
		return false
	}
}

func (u *UnionIterator) Seek(to RowNumberWithDefinitionLevel) bool {
	to.RowNumber = TruncateRowNumber(to)
	for iterNum, iter := range u.iters {
		if p := u.peeks[iterNum]; p == nil || CompareRowNumbers(to.DefinitionLevel, p.RowNumber, to.RowNumber) == -1 {
			if iter.Seek(to) {
				u.peeks[iterNum] = iter.At()
			} else {
				u.peeks[iterNum] = nil
			}
		}
	}
	return u.Next()
}

func (u *UnionIterator) peek(iterNum int) *IteratorResult {
	if u.peeks[iterNum] == nil {
		if u.iters[iterNum].Next() {
			u.peeks[iterNum] = u.iters[iterNum].At()
		}
	}
	return u.peeks[iterNum]
}

// Collect data from the given iterators until they point at
// the next row (according to the configured definition level)
// or are exhausted.
func (u *UnionIterator) collect(iterNums []int, rowNumber RowNumber) *IteratorResult {
	result := iteratorResultPoolGet()
	result.RowNumber = rowNumber

	for _, iterNum := range iterNums {
		for u.peeks[iterNum] != nil && CompareRowNumbers(u.definitionLevel, u.peeks[iterNum].RowNumber, rowNumber) == 0 {

			result.Append(u.peeks[iterNum])

			iteratorResultPoolPut(u.peeks[iterNum])

			if u.iters[iterNum].Next() {
				u.peeks[iterNum] = u.iters[iterNum].At()
			}

		}
	}

	return result
}

func (u *UnionIterator) Err() error {
	for _, i := range u.iters {
		if err := i.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (u *UnionIterator) Close() error {
	var merr multierror.MultiError
	for _, i := range u.iters {
		merr.Add(i.Close())
	}
	return merr.Err()
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
	buffer [][]parquet.Value
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

type RowGetter interface {
	RowNumber() int64
}

type RowNumberIterator[T any] struct {
	iter.Iterator[T]
	current *IteratorResult
	err     error
}

func NewRowNumberIterator[T any](iter iter.Iterator[T]) *RowNumberIterator[T] {
	return &RowNumberIterator[T]{
		Iterator: iter,
	}
}

func (r *RowNumberIterator[T]) Next() bool {
	if !r.Iterator.Next() {
		return false
	}
	r.current = iteratorResultPoolGet()
	r.current.Reset()
	rowGetter, ok := any(r.Iterator.At()).(RowGetter)
	if !ok {
		if r.err == nil {
			r.err = fmt.Errorf("row number iterator: %T does not implement RowGetter", r.Iterator.At())
		}
		return false
	}
	r.current.RowNumber = RowNumber{rowGetter.RowNumber(), -1, -1, -1, -1, -1}
	r.current.Entries = append(r.current.Entries, struct {
		k        string
		V        parquet.Value
		RowValue interface{}
	}{
		RowValue: r.Iterator.At(),
	})
	return true
}

func (r *RowNumberIterator[T]) At() *IteratorResult {
	return r.current
}

func (r *RowNumberIterator[T]) Err() error {
	if r.err != nil {
		return r.err
	}
	return r.Iterator.Err()
}

func (r *RowNumberIterator[T]) Seek(to RowNumberWithDefinitionLevel) bool {
	for CompareRowNumbers(0, r.current.RowNumber, to.RowNumber) == -1 {
		if !r.Next() {
			return false
		}
	}
	return true
}

// SyncIterator is a synchronous column iterator. It scans through the given row
// groups and column, and applies the optional predicate to each chunk, page, and value.
// Results are read by calling Next() until it returns nil.
type SyncIterator struct {
	// Config
	column     int
	columnName string
	table      string
	rgs        []parquet.RowGroup
	rgsMin     []RowNumber
	rgsMax     []RowNumber // Exclusive, row number of next one past the row group
	readSize   int
	selectAs   string
	filter     *InstrumentedPredicate

	// Status
	ctx             context.Context
	cancel          func()
	span            opentracing.Span
	metrics         *Metrics
	curr            RowNumber
	currRowGroup    parquet.RowGroup
	currRowGroupMin RowNumber
	currRowGroupMax RowNumber
	currChunk       parquet.ColumnChunk
	currPages       parquet.Pages
	currPage        parquet.Page
	currPageMax     RowNumber
	currValues      parquet.ValueReader
	currBuf         []parquet.Value
	currBufN        int

	err error
	res *IteratorResult
}

var _ Iterator = (*SyncIterator)(nil)

var syncIteratorPool = sync.Pool{
	New: func() interface{} {
		return []parquet.Value{}
	},
}

func syncIteratorPoolGet(capacity, len int) []parquet.Value {
	res := syncIteratorPool.Get().([]parquet.Value)
	if cap(res) < capacity {
		res = make([]parquet.Value, capacity)
	}
	res = res[:len]
	return res
}

func syncIteratorPoolPut(b []parquet.Value) {
	for i := range b {
		b[i] = parquet.Value{}
	}
	syncIteratorPool.Put(b) // nolint: staticcheck
}

func NewSyncIterator(ctx context.Context, rgs []parquet.RowGroup, column int, columnName string, readSize int, filter Predicate, selectAs string) *SyncIterator {

	// Assign row group bounds.
	// Lower bound is inclusive
	// Upper bound is exclusive, points at the first row of the next group
	rn := EmptyRowNumber()
	rgsMin := make([]RowNumber, len(rgs))
	rgsMax := make([]RowNumber, len(rgs))
	for i, rg := range rgs {
		rgsMin[i] = rn
		rgsMax[i] = rn
		rgsMax[i].Skip(rg.NumRows() + 1)
		rn.Skip(rg.NumRows())
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, "syncIterator", opentracing.Tags{
		"columnIndex": column,
		"column":      columnName,
	})

	ctx, cancel := context.WithCancel(ctx)

	return &SyncIterator{
		table:      strings.ToLower(rgs[0].Schema().Name()) + "s",
		ctx:        ctx,
		cancel:     cancel,
		metrics:    getMetricsFromContext(ctx),
		span:       span,
		column:     column,
		columnName: columnName,
		rgs:        rgs,
		readSize:   readSize,
		selectAs:   selectAs,
		rgsMin:     rgsMin,
		rgsMax:     rgsMax,
		filter:     &InstrumentedPredicate{pred: filter},
		curr:       EmptyRowNumber(),
	}
}

func (c *SyncIterator) At() *IteratorResult {
	return c.res
}

func (c *SyncIterator) Next() bool {
	rn, v, err := c.next()
	if err != nil {
		c.res = nil
		c.err = err
		return false
	}
	if !rn.Valid() {
		c.res = nil
		c.err = nil
		return false
	}
	c.res = c.makeResult(rn, v)
	return true
}

// SeekTo moves this iterator to the next result that is greater than
// or equal to the given row number (and based on the given definition level)
func (c *SyncIterator) Seek(to RowNumberWithDefinitionLevel) bool {

	if c.seekRowGroup(to.RowNumber, to.DefinitionLevel) {
		c.res = nil
		c.err = nil
		return false
	}

	done, err := c.seekPages(to.RowNumber, to.DefinitionLevel)
	if err != nil {
		c.res = nil
		c.err = err
		return false
	}
	if done {
		c.res = nil
		c.err = nil
		return false
	}

	// The row group and page have been selected to where this value is possibly
	// located. Now scan through the page and look for it.
	for {
		rn, v, err := c.next()
		if err != nil {
			c.res = nil
			c.err = err
			return false
		}
		if !rn.Valid() {
			c.res = nil
			c.err = nil
			return false
		}

		if CompareRowNumbers(to.DefinitionLevel, rn, to.RowNumber) >= 0 {
			c.res = c.makeResult(rn, v)
			c.err = nil
			return true
		}
	}
}

func (c *SyncIterator) popRowGroup() (parquet.RowGroup, RowNumber, RowNumber) {
	if len(c.rgs) == 0 {
		return nil, EmptyRowNumber(), EmptyRowNumber()
	}

	rg := c.rgs[0]
	min := c.rgsMin[0]
	max := c.rgsMax[0]

	c.rgs = c.rgs[1:]
	c.rgsMin = c.rgsMin[1:]
	c.rgsMax = c.rgsMax[1:]

	return rg, min, max
}

// seekRowGroup skips ahead to the row group that could contain the value at the
// desired row number. Does nothing if the current row group is already the correct one.
func (c *SyncIterator) seekRowGroup(seekTo RowNumber, definitionLevel int) (done bool) {
	if c.currRowGroup != nil && CompareRowNumbers(definitionLevel, seekTo, c.currRowGroupMax) >= 0 {
		// Done with this row group
		c.closeCurrRowGroup()
	}

	for c.currRowGroup == nil {

		rg, min, max := c.popRowGroup()
		if rg == nil {
			return true
		}

		if CompareRowNumbers(definitionLevel, seekTo, max) != -1 {
			continue
		}

		cc := rg.ColumnChunks()[c.column]
		if c.filter != nil && !c.filter.KeepColumnChunk(cc) {
			continue
		}

		// This row group matches both row number and filter.
		c.setRowGroup(rg, min, max)
	}

	return c.currRowGroup == nil
}

// seekPages skips ahead in the current row group to the page that could contain the value at
// the desired row number. Does nothing if the current page is already the correct one.
func (c *SyncIterator) seekPages(seekTo RowNumber, definitionLevel int) (done bool, err error) {
	if c.currPage != nil && CompareRowNumbers(definitionLevel, seekTo, c.currPageMax) >= 0 {
		// Value not in this page
		c.setPage(nil)
	}

	if c.currPage == nil {

		// TODO (mdisibio)   :((((((((
		//    pages.SeekToRow is more costly than expected.  It doesn't reuse existing i/o
		// so it can't be called naively every time we swap pages. We need to figure out
		// a way to determine when it is worth calling here.
		/*
			// Seek into the pages. This is relative to the start of the row group
			if seekTo[0] > 0 {
				// Determine row delta. We subtract 1 because curr points at the previous row
				skip := seekTo[0] - c.currRowGroupMin[0] - 1
				if skip > 0 {
					if err := c.currPages.SeekToRow(skip); err != nil {
						return true, err
					}
					c.curr.Skip(skip)
				}
			}*/

		for c.currPage == nil {
			pg, err := c.currPages.ReadPage()
			if pg == nil || err != nil {
				// No more pages in this column chunk,
				// cleanup and exit.
				if err == io.EOF {
					err = nil
				}
				parquet.Release(pg)
				c.closeCurrRowGroup()
				return true, err
			}
			c.metrics.pageReadsTotal.WithLabelValues(c.table, c.columnName).Add(1)
			c.span.LogFields(
				log.String("msg", "reading page (seekPages)"),
				log.Int64("page_num_values", pg.NumValues()),
				log.Int64("page_size", pg.Size()),
			)

			// Skip based on row number?
			newRN := c.curr
			newRN.Skip(pg.NumRows() + 1)
			if CompareRowNumbers(definitionLevel, seekTo, newRN) >= 0 {
				c.curr.Skip(pg.NumRows())
				parquet.Release(pg)
				continue
			}

			// Skip based on filter?
			if c.filter != nil && !c.filter.KeepPage(pg) {
				c.curr.Skip(pg.NumRows())
				parquet.Release(pg)
				continue
			}

			c.setPage(pg)
		}
	}

	return false, nil
}

// next is the core functionality of this iterator and returns the next matching result. This
// may involve inspecting multiple row groups, pages, and values until a match is found. When
// we run out of things to inspect, it returns nil. The reason this method is distinct from
// Next() is because it doesn't wrap the results in an IteratorResult, which is more efficient
// when being called multiple times and throwing away the results like in SeekTo().
func (c *SyncIterator) next() (RowNumber, *parquet.Value, error) {
	for {

		// return if context is cancelled
		select {
		case <-c.ctx.Done():
			return EmptyRowNumber(), nil, c.ctx.Err()
		default:
		}

		if c.currRowGroup == nil {
			rg, min, max := c.popRowGroup()
			if rg == nil {
				return EmptyRowNumber(), nil, nil
			}

			cc := rg.ColumnChunks()[c.column]
			if c.filter != nil && !c.filter.KeepColumnChunk(cc) {
				continue
			}

			c.setRowGroup(rg, min, max)
		}

		if c.currPage == nil {
			pg, err := c.currPages.ReadPage()
			if pg == nil || err == io.EOF {
				// This row group is exhausted
				c.closeCurrRowGroup()
				continue
			}
			if err != nil {
				return EmptyRowNumber(), nil, err
			}
			c.metrics.pageReadsTotal.WithLabelValues(c.table, c.columnName).Add(1)
			c.span.LogFields(
				log.String("msg", "reading page (next)"),
				log.Int64("page_num_values", pg.NumValues()),
				log.Int64("page_size", pg.Size()),
			)

			if c.filter != nil && !c.filter.KeepPage(pg) {
				// This page filtered out
				c.curr.Skip(pg.NumRows())
				parquet.Release(pg)
				continue
			}
			c.setPage(pg)
		}

		// Read next batch of values if needed
		if c.currBuf == nil {
			c.currBuf = syncIteratorPoolGet(c.readSize, 0)
		}
		if c.currBufN >= len(c.currBuf) || len(c.currBuf) == 0 {
			c.currBuf = c.currBuf[:cap(c.currBuf)]
			n, err := c.currValues.ReadValues(c.currBuf)
			if err != nil && err != io.EOF {
				return EmptyRowNumber(), nil, err
			}
			c.currBuf = c.currBuf[:n]
			c.currBufN = 0
			if n == 0 {
				// This value reader and page are exhausted.
				c.setPage(nil)
				continue
			}
		}

		// Consume current buffer until empty
		for c.currBufN < len(c.currBuf) {
			v := &c.currBuf[c.currBufN]

			// Inspect all values to track the current row number,
			// even if the value is filtered out next.
			c.curr.Next(v.RepetitionLevel(), v.DefinitionLevel())
			c.currBufN++

			if c.filter != nil && !c.filter.KeepValue(*v) {
				continue
			}

			return c.curr, v, nil
		}
	}
}

func (c *SyncIterator) setRowGroup(rg parquet.RowGroup, min, max RowNumber) {
	c.closeCurrRowGroup()
	c.curr = min
	c.currRowGroup = rg
	c.currRowGroupMin = min
	c.currRowGroupMax = max
	c.currChunk = rg.ColumnChunks()[c.column]
	c.currPages = c.currChunk.Pages()
}

func (c *SyncIterator) setPage(pg parquet.Page) {

	// Handle an outgoing page
	if c.currPage != nil {
		c.curr = c.currPageMax.Preceding() // Reposition current row number to end of this page.
		parquet.Release(c.currPage)
		c.currPage = nil
	}

	// Reset value buffers
	c.currValues = nil
	c.currPageMax = EmptyRowNumber()
	c.currBufN = 0

	// If we don't immediately have a new incoming page
	// then return the buffer to the pool.
	if pg == nil && c.currBuf != nil {
		syncIteratorPoolPut(c.currBuf)
		c.currBuf = nil
	}

	// Handle an incoming page
	if pg != nil {
		rn := c.curr
		rn.Skip(pg.NumRows() + 1) // Exclusive upper bound, points at the first rownumber in the next page
		c.currPage = pg
		c.currPageMax = rn
		c.currValues = pg.Values()
	}
}

func (c *SyncIterator) closeCurrRowGroup() {
	if c.currPages != nil {
		c.currPages.Close()
	}

	c.currRowGroup = nil
	c.currRowGroupMin = EmptyRowNumber()
	c.currRowGroupMax = EmptyRowNumber()
	c.currChunk = nil
	c.currPages = nil
	c.setPage(nil)
}

func (c *SyncIterator) makeResult(t RowNumber, v *parquet.Value) *IteratorResult {
	r := iteratorResultPoolGet()
	r.RowNumber = t
	if c.selectAs != "" {
		r.AppendValue(c.selectAs, v.Clone())
	}
	return r
}

func (c *SyncIterator) Err() error {
	return c.err
}

func (c *SyncIterator) Close() error {
	c.cancel()
	c.closeCurrRowGroup()

	c.span.SetTag("inspectedColumnChunks", c.filter.InspectedColumnChunks.Load())
	c.span.SetTag("inspectedPages", c.filter.InspectedPages.Load())
	c.span.SetTag("inspectedValues", c.filter.InspectedValues.Load())
	c.span.SetTag("keptColumnChunks", c.filter.KeptColumnChunks.Load())
	c.span.SetTag("keptPages", c.filter.KeptPages.Load())
	c.span.SetTag("keptValues", c.filter.KeptValues.Load())
	c.span.Finish()
	return nil
}
