package querybackend

import (
	"fmt"
	"io"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/v2/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/tsdb/index"
)

// As we expect rows to be very small, we want to fetch a bigger
// batch of rows at once to amortize the latency of reading.
const bigBatchSize = 2 << 10

const profileEntryColumnReadSize = 1024

type ProfileEntry struct {
	RowNum      int64
	Timestamp   model.Time
	Fingerprint model.Fingerprint
	Labels      phlaremodel.Labels
	Partition   uint64
	ID          string
}

func (e ProfileEntry) RowNumber() int64 { return e.RowNum }

type profileIteratorOption struct {
	iterator func(*iteratorOpts)
	series   func(*seriesOpts)
}

func withAllLabels() profileIteratorOption {
	return profileIteratorOption{
		series: func(opts *seriesOpts) {
			opts.allLabels = true
		},
	}
}

func withGroupByLabels(by ...string) profileIteratorOption {
	return profileIteratorOption{
		series: func(opts *seriesOpts) {
			opts.groupBy = by
		},
	}
}

func withFetchPartition(v bool) profileIteratorOption {
	return profileIteratorOption{
		iterator: func(opts *iteratorOpts) {
			opts.fetchPartition = v
		},
	}
}

func withFetchProfileIDs(v bool) profileIteratorOption {
	return profileIteratorOption{
		iterator: func(opts *iteratorOpts) {
			opts.fetchProfileIDs = v
		},
	}
}

func withProfileIDSelector(ids ...string) (profileIteratorOption, error) {
	// convert profile ids into uuids
	uuids := make([]string, 0, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			return profileIteratorOption{}, err
		}
		uuids = append(uuids, string(u[:]))
	}

	return profileIteratorOption{
		iterator: func(opts *iteratorOpts) {
			opts.profileIDSelector = uuids
		},
	}, nil
}

type iteratorOpts struct {
	profileIDSelector []string // this is a slice of the byte form of the UUID
	fetchProfileIDs   bool
	fetchPartition    bool
}

func iteratorOptsFromOptions(options []profileIteratorOption) iteratorOpts {
	opts := iteratorOpts{
		fetchPartition: true,
	}
	for _, f := range options {
		if f.iterator != nil {
			f.iterator(&opts)
		}
	}
	return opts
}

func profileEntryIterator(q *queryContext, options ...profileIteratorOption) (iter.Iterator[ProfileEntry], error) {
	opts := iteratorOptsFromOptions(options)

	series, err := getSeries(q.ds.Index(), q.req.matchers, options...)
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return iter.NewSliceIterator[ProfileEntry](nil), nil
	}

	// Profile metadata lives in a small set of top-level parquet columns. Scan the
	// predicate columns into bitmaps first, then project only accepted rows. This
	// keeps the query path columnar and avoids the generic row-oriented join stack.
	scanner := profileEntryColumnScanner{
		q:           q,
		int32Values: make([]int32, profileEntryColumnReadSize),
		int64Values: make([]int64, profileEntryColumnReadSize),
	}
	acceptedRows, err := selectProfileEntryRows(&scanner, series, opts)
	if err != nil {
		return nil, err
	}
	if acceptedRows.IsEmpty() {
		return iter.NewSliceIterator[ProfileEntry](nil), nil
	}

	entries, err := materializeProfileEntries(&scanner, acceptedRows, series, opts)
	if err != nil {
		return nil, err
	}
	return iter.NewSliceIterator(entries), nil
}

func selectProfileEntryRows(scanner *profileEntryColumnScanner, series map[uint32]series, opts iteratorOpts) (*roaring.Bitmap, error) {
	timePredicate := parquetquery.NewIntBetweenPredicate(scanner.q.req.startTime, scanner.q.req.endTime)
	timeRows, err := scanner.matchingInt64Rows(schemav1.TimeNanosColumnName, timePredicate, func(value int64) bool {
		return scanner.q.req.startTime <= value && value <= scanner.q.req.endTime
	})
	if err != nil {
		return nil, err
	}
	if timeRows.IsEmpty() {
		return timeRows, nil
	}

	seriesPredicate := parquetquery.NewMapPredicate(series)
	acceptedRows, err := scanner.matchingInt32Rows(schemav1.SeriesIndexColumnName, seriesPredicate, func(value int32) bool {
		_, ok := series[uint32(value)]
		return ok
	})
	if err != nil {
		return nil, err
	}
	acceptedRows.And(timeRows)

	if len(opts.profileIDSelector) > 0 && !acceptedRows.IsEmpty() {
		idPredicate := parquetquery.NewStringInPredicate(opts.profileIDSelector)
		profileIDs := profileIDSelectorSet(opts.profileIDSelector)
		profileIDRows, err := scanner.matchingFixedLenByteArrayRows(schemav1.IDColumnName, idPredicate, func(value []byte) bool {
			if len(value) != 16 {
				return false
			}
			var id [16]byte
			copy(id[:], value)
			_, ok := profileIDs[id]
			return ok
		})
		if err != nil {
			return nil, err
		}
		acceptedRows.And(profileIDRows)
	}

	return acceptedRows, nil
}

func materializeProfileEntries(scanner *profileEntryColumnScanner, acceptedRows *roaring.Bitmap, series map[uint32]series, opts iteratorOpts) ([]ProfileEntry, error) {
	entries := make([]ProfileEntry, 0, acceptedRows.GetCardinality())
	// Build entries from SeriesIndex first. Subsequent scans over top-level
	// scalar columns visit rows in the same order, so their accepted-row ordinal
	// maps directly to the entry slice index.
	if err := scanner.scanInt32(schemav1.SeriesIndexColumnName, parquetquery.NewMapPredicate(series), func(row uint32, value int32) error {
		if !acceptedRows.Contains(row) {
			return nil
		}
		x := series[uint32(value)]
		entries = append(entries, ProfileEntry{
			RowNum:      int64(row),
			Fingerprint: x.fingerprint,
			Labels:      x.labels,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	if err := scanAcceptedRowsInt64Column(scanner, acceptedRows, schemav1.TimeNanosColumnName, func(i int, value int64) error {
		entries[i].Timestamp = model.TimeFromUnixNano(value)
		return nil
	}); err != nil {
		return nil, err
	}

	if opts.fetchPartition {
		if err := scanAcceptedRowsInt64Column(scanner, acceptedRows, schemav1.StacktracePartitionColumnName, func(i int, value int64) error {
			entries[i].Partition = uint64(value)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	if opts.fetchProfileIDs {
		var u uuid.UUID
		if err := scanAcceptedRowsFixedLenByteArrayColumn(scanner, acceptedRows, schemav1.IDColumnName, func(i int, value []byte) error {
			if len(value) != 16 {
				return nil
			}
			copy(u[:], value)
			entries[i].ID = u.String()
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func scanAcceptedRowsInt64Column(scanner *profileEntryColumnScanner, acceptedRows *roaring.Bitmap, columnName string, fn func(entryIndex int, value int64) error) error {
	entryIndex := 0
	return scanner.scanInt64(columnName, nil, func(row uint32, value int64) error {
		if !acceptedRows.Contains(row) {
			return nil
		}
		if err := fn(entryIndex, value); err != nil {
			return err
		}
		entryIndex++
		return nil
	})
}

func scanAcceptedRowsFixedLenByteArrayColumn(scanner *profileEntryColumnScanner, acceptedRows *roaring.Bitmap, columnName string, fn func(entryIndex int, value []byte) error) error {
	entryIndex := 0
	return scanner.scanFixedLenByteArray(columnName, nil, func(row uint32, value []byte) error {
		if !acceptedRows.Contains(row) {
			return nil
		}
		if err := fn(entryIndex, value); err != nil {
			return err
		}
		entryIndex++
		return nil
	})
}

type profileEntryColumnScanner struct {
	q           *queryContext
	values      []parquet.Value
	int32Values []int32
	int64Values []int64
	byteValues  []byte
}

func (s *profileEntryColumnScanner) matchingInt32Rows(columnName string, predicate parquetquery.Predicate, keep func(int32) bool) (*roaring.Bitmap, error) {
	rows := roaring.New()
	if err := s.scanInt32(columnName, predicate, func(row uint32, value int32) error {
		if keep(value) {
			rows.Add(row)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *profileEntryColumnScanner) matchingInt64Rows(columnName string, predicate parquetquery.Predicate, keep func(int64) bool) (*roaring.Bitmap, error) {
	rows := roaring.New()
	if err := s.scanInt64(columnName, predicate, func(row uint32, value int64) error {
		if keep(value) {
			rows.Add(row)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *profileEntryColumnScanner) matchingFixedLenByteArrayRows(columnName string, predicate parquetquery.Predicate, keep func([]byte) bool) (*roaring.Bitmap, error) {
	rows := roaring.New()
	if err := s.scanFixedLenByteArray(columnName, predicate, func(row uint32, value []byte) error {
		if keep(value) {
			rows.Add(row)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *profileEntryColumnScanner) scanInt32(columnName string, predicate parquetquery.Predicate, fn func(row uint32, value int32) error) error {
	return s.scanTypedColumn(columnName, predicate, func(page parquet.Page, firstRow int64) error {
		reader := page.Values()
		if typed, ok := reader.(parquet.Int32Reader); ok {
			return s.readInt32Page(typed, page.NumRows(), firstRow, fn)
		}
		return s.readValuePage(reader, page.NumRows(), firstRow, func(row uint32, value parquet.Value) error {
			return fn(row, int32(value.Uint32()))
		})
	})
}

func (s *profileEntryColumnScanner) scanInt64(columnName string, predicate parquetquery.Predicate, fn func(row uint32, value int64) error) error {
	return s.scanTypedColumn(columnName, predicate, func(page parquet.Page, firstRow int64) error {
		reader := page.Values()
		if typed, ok := reader.(parquet.Int64Reader); ok {
			return s.readInt64Page(typed, page.NumRows(), firstRow, fn)
		}
		return s.readValuePage(reader, page.NumRows(), firstRow, func(row uint32, value parquet.Value) error {
			return fn(row, value.Int64())
		})
	})
}

func (s *profileEntryColumnScanner) scanFixedLenByteArray(columnName string, predicate parquetquery.Predicate, fn func(row uint32, value []byte) error) error {
	return s.scanTypedColumn(columnName, predicate, func(page parquet.Page, firstRow int64) error {
		reader := page.Values()
		if typed, ok := reader.(parquet.FixedLenByteArrayReader); ok {
			return s.readFixedLenByteArrayPage(typed, page.Type().Length(), page.NumRows(), firstRow, fn)
		}
		return s.readValuePage(reader, page.NumRows(), firstRow, func(row uint32, value parquet.Value) error {
			return fn(row, value.Bytes())
		})
	})
}

// scanTypedColumn reads required top-level scalar columns page-by-page. The
// per-type scanners use parquet's native slice readers when available and only
// fall back to parquet.Value materialization for pages that do not expose them.
func (s *profileEntryColumnScanner) scanTypedColumn(columnName string, predicate parquetquery.Predicate, readPage func(page parquet.Page, firstRow int64) error) error {
	columnIndex, _ := parquetquery.GetColumnIndexByPath(s.q.ds.Profiles().Root(), columnName)
	if columnIndex < 0 {
		return fmt.Errorf("column %s not found in profile parquet table", columnName)
	}

	var rowBase int64
	for _, rowGroup := range s.q.ds.Profiles().RowGroups() {
		if err := s.q.ctx.Err(); err != nil {
			return err
		}
		columnChunk := rowGroup.ColumnChunks()[columnIndex]
		if predicate != nil {
			columnChunkIndex, err := columnChunk.ColumnIndex()
			if err != nil {
				return err
			}
			if !predicate.KeepColumnChunk(columnChunkIndex) {
				rowBase += rowGroup.NumRows()
				continue
			}
		}

		pages := columnChunk.Pages()
		var rowInGroup int64
		for {
			page, err := pages.ReadPage()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = pages.Close()
				return err
			}
			if page == nil {
				break
			}

			pageRows := page.NumRows()
			if predicate != nil && !predicate.KeepPage(page) {
				rowInGroup += pageRows
				parquet.Release(page)
				continue
			}

			if err := readPage(page, rowBase+rowInGroup); err != nil {
				parquet.Release(page)
				_ = pages.Close()
				return err
			}
			rowInGroup += pageRows
			parquet.Release(page)
		}
		if err := pages.Close(); err != nil {
			return err
		}
		rowBase += rowGroup.NumRows()
	}
	return nil
}

func (s *profileEntryColumnScanner) readInt32Page(reader parquet.Int32Reader, pageRows, firstRow int64, fn func(row uint32, value int32) error) error {
	var rowInPage int64
	for rowInPage < pageRows {
		n, err := reader.ReadInt32s(s.int32Values)
		for i := 0; i < n; i++ {
			row, rowErr := profileEntryRowNumber(firstRow + rowInPage)
			if rowErr != nil {
				return rowErr
			}
			if err := fn(row, s.int32Values[i]); err != nil {
				return err
			}
			rowInPage++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *profileEntryColumnScanner) readInt64Page(reader parquet.Int64Reader, pageRows, firstRow int64, fn func(row uint32, value int64) error) error {
	var rowInPage int64
	for rowInPage < pageRows {
		n, err := reader.ReadInt64s(s.int64Values)
		for i := 0; i < n; i++ {
			row, rowErr := profileEntryRowNumber(firstRow + rowInPage)
			if rowErr != nil {
				return rowErr
			}
			if err := fn(row, s.int64Values[i]); err != nil {
				return err
			}
			rowInPage++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *profileEntryColumnScanner) readFixedLenByteArrayPage(reader parquet.FixedLenByteArrayReader, valueWidth int, pageRows, firstRow int64, fn func(row uint32, value []byte) error) error {
	if valueWidth <= 0 {
		return fmt.Errorf("fixed-length byte array width must be positive")
	}
	if cap(s.byteValues) < profileEntryColumnReadSize*valueWidth {
		s.byteValues = make([]byte, profileEntryColumnReadSize*valueWidth)
	}
	buffer := s.byteValues[:profileEntryColumnReadSize*valueWidth]

	var rowInPage int64
	for rowInPage < pageRows {
		n, err := reader.ReadFixedLenByteArrays(buffer)
		for i := 0; i < n; i++ {
			row, rowErr := profileEntryRowNumber(firstRow + rowInPage)
			if rowErr != nil {
				return rowErr
			}
			start := i * valueWidth
			if err := fn(row, buffer[start:start+valueWidth]); err != nil {
				return err
			}
			rowInPage++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *profileEntryColumnScanner) readValuePage(reader parquet.ValueReader, pageRows, firstRow int64, fn func(row uint32, value parquet.Value) error) error {
	if s.values == nil {
		s.values = make([]parquet.Value, profileEntryColumnReadSize)
	}
	var rowInPage int64
	for rowInPage < pageRows {
		n, err := reader.ReadValues(s.values)
		for i := 0; i < n; i++ {
			row, rowErr := profileEntryRowNumber(firstRow + rowInPage)
			if rowErr != nil {
				return rowErr
			}
			if err := fn(row, s.values[i]); err != nil {
				return err
			}
			rowInPage++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func profileIDSelectorSet(selectors []string) map[[16]byte]struct{} {
	ids := make(map[[16]byte]struct{}, len(selectors))
	for _, selector := range selectors {
		if len(selector) != 16 {
			continue
		}
		var id [16]byte
		copy(id[:], selector)
		ids[id] = struct{}{}
	}
	return ids
}

func profileEntryRowNumber(row int64) (uint32, error) {
	if row < 0 || row > int64(^uint32(0)) {
		return 0, fmt.Errorf("profile row number %d out of uint32 range", row)
	}
	return uint32(row), nil
}

type series struct {
	fingerprint model.Fingerprint
	labels      phlaremodel.Labels
}

type seriesOpts struct {
	allLabels bool // when this is true, groupBy is ignored
	groupBy   []string
}

func getSeries(reader phlaredb.IndexReader, matchers []*labels.Matcher, options ...profileIteratorOption) (map[uint32]series, error) {
	var opts seriesOpts
	for _, f := range options {
		if f.series != nil {
			f.series(&opts)
		}
	}

	postings, err := getPostings(reader, matchers...)
	if err != nil {
		return nil, err
	}
	chunks := make([]index.ChunkMeta, 1)
	s := make(map[uint32]series)
	l := make(phlaremodel.Labels, 0, 6)
	for postings.Next() {
		var fp uint64
		if opts.allLabels {
			fp, err = reader.Series(postings.At(), &l, &chunks)
		} else {
			fp, err = reader.SeriesBy(postings.At(), &l, &chunks, opts.groupBy...)
		}
		if err != nil {
			return nil, err
		}
		_, ok := s[chunks[0].SeriesIndex]
		if ok {
			continue
		}
		s[chunks[0].SeriesIndex] = series{
			fingerprint: model.Fingerprint(fp),
			labels:      l.Clone(),
		}
	}
	return s, postings.Err()
}

func getPostings(reader phlaredb.IndexReader, matchers ...*labels.Matcher) (index.Postings, error) {
	if len(matchers) == 0 {
		k, v := index.AllPostingsKey()
		return reader.Postings(k, nil, v)
	}
	return phlaredb.PostingsForMatchers(reader, nil, matchers...)
}

func getSeriesIDs(reader phlaredb.IndexReader, matchers ...*labels.Matcher) (map[uint32]struct{}, error) {
	postings, err := getPostings(reader, matchers...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = postings.Close()
	}()
	visited := make(map[uint32]struct{})
	chunks := make([]index.ChunkMeta, 1)
	for postings.Next() {
		if _, err = reader.Series(postings.At(), nil, &chunks); err != nil {
			return nil, err
		}
		visited[chunks[0].SeriesIndex] = struct{}{}
	}
	if err = postings.Err(); err != nil {
		return nil, err
	}
	return visited, nil
}
