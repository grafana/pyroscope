package query_backend

import (
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
}

func (e ProfileEntry) RowNumber() int64 { return e.RowNum }

func profileEntryIterator(q *queryContext, groupBy ...string) (iter.Iterator[ProfileEntry], error) {
	series, err := getSeriesLabels(q.ds.Index(), q.req.matchers, groupBy...)
	if err != nil {
		return nil, err
	}
	results := parquetquery.NewBinaryJoinIterator(0,
		q.ds.Profiles().Column(q.ctx, "SeriesIndex", parquetquery.NewMapPredicate(series)),
		q.ds.Profiles().Column(q.ctx, "TimeNanos", parquetquery.NewIntBetweenPredicate(q.req.startTime, q.req.endTime)),
	)
	results = parquetquery.NewBinaryJoinIterator(0, results,
		q.ds.Profiles().Column(q.ctx, "StacktracePartition", nil),
	)

	buf := make([][]parquet.Value, 3)
	entries := iter.NewAsyncBatchIterator[*parquetquery.IteratorResult, ProfileEntry](
		results, bigBatchSize,
		func(r *parquetquery.IteratorResult) ProfileEntry {
			buf = r.Columns(buf,
				schemav1.SeriesIndexColumnName,
				schemav1.TimeNanosColumnName,
				schemav1.StacktracePartitionColumnName)
			x := series[buf[0][0].Uint32()]
			return ProfileEntry{
				RowNum:      r.RowNumber[0],
				Timestamp:   model.TimeFromUnixNano(buf[1][0].Int64()),
				Fingerprint: x.fingerprint,
				Labels:      x.labels,
				Partition:   buf[2][0].Uint64(),
			}
		},
		func([]ProfileEntry) {},
	)
	return entries, nil
}

type seriesLabels struct {
	fingerprint model.Fingerprint
	labels      phlaremodel.Labels
}

func getSeriesLabels(reader phlaredb.IndexReader, matchers []*labels.Matcher, by ...string) (map[uint32]seriesLabels, error) {
	postings, err := getPostings(reader, matchers...)
	if err != nil {
		return nil, err
	}
	chunks := make([]index.ChunkMeta, 1)
	series := make(map[uint32]seriesLabels)
	l := make(phlaremodel.Labels, 0, 6)
	for postings.Next() {
		fp, err := reader.SeriesBy(postings.At(), &l, &chunks, by...)
		if err != nil {
			return nil, err
		}
		_, ok := series[chunks[0].SeriesIndex]
		if ok {
			continue
		}
		series[chunks[0].SeriesIndex] = seriesLabels{
			fingerprint: model.Fingerprint(fp),
			labels:      l.Clone(),
		}
	}

	return series, postings.Err()
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
