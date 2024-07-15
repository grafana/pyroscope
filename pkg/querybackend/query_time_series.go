package querybackend

import (
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_TIME_SERIES,
		querybackendv1.ReportType_REPORT_TIME_SERIES,
		queryTimeSeries,
		newTimeSeriesMerger,
		[]section{
			sectionTSDB,
			sectionProfiles,
		}...,
	)
}

func queryTimeSeries(q *queryContext, query *querybackendv1.Query) (r *querybackendv1.Report, err error) {
	entries, err := q.profileEntryIterator(query.TimeSeries.GroupBy...)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	column, err := schemav1.ResolveColumnByPath(q.svc.profiles.Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return nil, err
	}

	rows := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.svc.profiles.RowGroups(), column.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	builder := phlaremodel.NewTimeSeriesBuilder(query.TimeSeries.GroupBy...)
	for rows.Next() {
		row := rows.At()
		builder.Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
		)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	resp := &querybackendv1.Report{
		TimeSeries: &querybackendv1.TimeSeriesReport{
			Query:      query.TimeSeries.CloneVT(),
			TimeSeries: builder.Build(),
		},
	}

	return resp, nil
}

func (q *queryContext) profileEntryIterator(groupBy ...string) (iter.Iterator[ProfileEntry], error) {
	series, err := getSeriesLabels(q.svc.tsdb, q.req.matchers, groupBy...)
	if err != nil {
		return nil, err
	}
	results := parquetquery.NewBinaryJoinIterator(0,
		q.svc.profiles.Column(q.ctx, "SeriesIndex", parquetquery.NewMapPredicate(series)),
		q.svc.profiles.Column(q.ctx, "TimeNanos", parquetquery.NewIntBetweenPredicate(q.req.startTime, q.req.endTime)),
	)

	buf := make([][]parquet.Value, 2)
	entries := iter.NewAsyncBatchIterator[*parquetquery.IteratorResult, ProfileEntry](
		results, 1<<10,
		func(r *parquetquery.IteratorResult) ProfileEntry {
			buf = r.Columns(buf,
				schemav1.SeriesIndexColumnName,
				schemav1.TimeNanosColumnName)
			x := series[buf[0][0].Uint32()]
			return ProfileEntry{
				RowNum:      r.RowNumber[0],
				Timestamp:   model.TimeFromUnixNano(buf[1][0].Int64()),
				Fingerprint: x.fingerprint,
				Labels:      x.labels,
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

type timeSeriesMerger struct {
	init   sync.Once
	query  *querybackendv1.TimeSeriesQuery
	series *phlaremodel.TimeSeriesMerger
}

func newTimeSeriesMerger() reportMerger { return new(timeSeriesMerger) }

func (m *timeSeriesMerger) merge(report *querybackendv1.Report) error {
	r := report.TimeSeries
	m.init.Do(func() {
		sum := r.Query.GetAggregation() == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
		m.series = phlaremodel.NewTimeSeriesMerger(sum)
		m.query = r.Query.CloneVT()
	})
	m.series.MergeTimeSeries(r.TimeSeries)
	return nil
}

func (m *timeSeriesMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		TimeSeries: &querybackendv1.TimeSeriesReport{
			Query:      m.query,
			TimeSeries: m.series.TimeSeries(),
		},
	}
}
