package querybackend

import (
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func profileEntryIterator(q *queryContext, groupBy ...string) (iter.Iterator[ProfileEntry], error) {
	series, err := getSeriesLabels(q.svc.TSDB, q.req.matchers, groupBy...)
	if err != nil {
		return nil, err
	}
	results := parquetquery.NewBinaryJoinIterator(0,
		q.svc.Profiles.Column(q.ctx, "SeriesIndex", parquetquery.NewMapPredicate(series)),
		q.svc.Profiles.Column(q.ctx, "TimeNanos", parquetquery.NewIntBetweenPredicate(q.req.startTime, q.req.endTime)),
	)
	results = parquetquery.NewBinaryJoinIterator(0, results,
		q.svc.Profiles.Column(q.ctx, "StacktracePartition", nil),
	)

	buf := make([][]parquet.Value, 3)
	entries := iter.NewAsyncBatchIterator[*parquetquery.IteratorResult, ProfileEntry](
		results, 128,
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

type ProfileEntry struct {
	RowNum      int64
	Timestamp   model.Time
	Fingerprint model.Fingerprint
	Labels      phlaremodel.Labels
	Partition   uint64
}

func (e ProfileEntry) RowNumber() int64 { return e.RowNum }

type seriesLabels struct {
	fingerprint model.Fingerprint
	labels      phlaremodel.Labels
}

func getSeriesLabels(reader *index.Reader, matchers []*labels.Matcher, by ...string) (map[uint32]seriesLabels, error) {
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
