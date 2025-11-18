package querybackend

import (
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

// As we expect rows to be very small, we want to fetch a bigger
// batch of rows at once to amortize the latency of reading.
const bigBatchSize = 2 << 10

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

type queryColumn struct {
	name      string
	predicate parquetquery.Predicate
	priority  int
}

type queryColumns []queryColumn

func (c queryColumns) names() []string {
	result := make([]string, len(c))
	for idx := range result {
		result[idx] = c[idx].name
	}
	return result
}

func (c queryColumns) join(q *queryContext) parquetquery.Iterator {
	var result parquetquery.Iterator

	// sort columns by priority, without modifying queryColumn slice
	order := make([]int, len(c))
	for idx := range order {
		order[idx] = idx
	}
	slices.SortFunc(order, func(a, b int) int {
		if r := c[a].priority - c[b].priority; r != 0 {
			return r
		}
		return strings.Compare(c[a].name, c[b].name)
	})

	for _, idx := range order {
		it := q.ds.Profiles().Column(q.ctx, c[idx].name, c[idx].predicate)
		if result == nil {
			result = it
			continue
		}
		result = parquetquery.NewBinaryJoinIterator(0,
			result,
			it,
		)
	}
	return result
}

func profileEntryIterator(q *queryContext, options ...profileIteratorOption) (iter.Iterator[ProfileEntry], error) {
	opts := iteratorOptsFromOptions(options)

	series, err := getSeries(q.ds.Index(), q.req.matchers, options...)
	if err != nil {
		return nil, err
	}

	columns := queryColumns{
		{schemav1.SeriesIndexColumnName, parquetquery.NewMapPredicate(series), 10},
		{schemav1.TimeNanosColumnName, parquetquery.NewIntBetweenPredicate(q.req.startTime, q.req.endTime), 15},
	}
	processor := []func([][]parquet.Value, *ProfileEntry){}

	// fetch partition if requested
	if opts.fetchPartition {
		offset := len(columns)
		columns = append(
			columns,
			queryColumn{schemav1.StacktracePartitionColumnName, nil, 20},
		)
		processor = append(processor, func(buf [][]parquet.Value, e *ProfileEntry) {
			e.Partition = buf[offset][0].Uint64()
		})
	}
	// fetch profile id if requested or part of the predicate
	if opts.fetchProfileIDs || len(opts.profileIDSelector) > 0 {
		var (
			predicate parquetquery.Predicate
			priority  = 20
		)
		if len(opts.profileIDSelector) > 0 {
			predicate = parquetquery.NewStringInPredicate(opts.profileIDSelector)
			priority = 5
		}
		offset := len(columns)
		columns = append(
			columns,
			queryColumn{schemav1.IDColumnName, predicate, priority},
		)
		var u uuid.UUID
		processor = append(processor, func(buf [][]parquet.Value, e *ProfileEntry) {
			b := buf[offset][0].Bytes()
			if len(b) != 16 {
				return
			}
			copy(u[:], b)
			e.ID = u.String()
		})
	}

	buf := make([][]parquet.Value, len(columns))
	columnNames := columns.names()

	entries := iter.NewAsyncBatchIterator[*parquetquery.IteratorResult, ProfileEntry](
		columns.join(q), bigBatchSize,
		func(r *parquetquery.IteratorResult) ProfileEntry {
			buf = r.Columns(buf, columnNames...)
			x := series[buf[0][0].Uint32()]
			e := ProfileEntry{
				RowNum:      r.RowNumber[0],
				Timestamp:   model.TimeFromUnixNano(buf[1][0].Int64()),
				Fingerprint: x.fingerprint,
				Labels:      x.labels,
			}
			for _, proc := range processor {
				proc(buf, &e)
			}
			return e
		},
		func([]ProfileEntry) {},
	)
	return entries, nil
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
