package querybackend

import (
	"sync"

	"github.com/prometheus/prometheus/model/labels"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_SERIES_LABELS,
		querybackendv1.ReportType_REPORT_SERIES_LABELS,
		querySeriesLabels,
		newSeriesLabelsMerger,
		[]section{sectionTSDB}...,
	)
}

func querySeriesLabels(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	postings, err := getPostings(q.svc.tsdb, q.req.matchers...)
	if err != nil {
		return nil, err
	}
	var tmp model.Labels
	var c []index.ChunkMeta
	l := make(map[uint64]model.Labels)
	for postings.Next() {
		fp, _ := q.svc.tsdb.SeriesBy(postings.At(), &tmp, &c, query.SeriesLabels.LabelNames...)
		if _, ok := l[fp]; ok {
			continue
		}
		l[fp] = tmp.Clone()
	}
	if err = postings.Err(); err != nil {
		return nil, err
	}
	series := make([]*typesv1.Labels, len(l))
	var i int
	for _, s := range l {
		series[i] = &typesv1.Labels{Labels: s}
		i++
	}
	resp := &querybackendv1.Report{
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query:        query.SeriesLabels.CloneVT(),
			SeriesLabels: series,
		},
	}
	return resp, nil
}

func getPostings(reader *index.Reader, matchers ...*labels.Matcher) (index.Postings, error) {
	if len(matchers) == 0 {
		k, v := index.AllPostingsKey()
		return reader.Postings(k, nil, v)
	}
	return phlaredb.PostingsForMatchers(reader, nil, matchers...)
}

type seriesLabelsMerger struct {
	init   sync.Once
	query  *querybackendv1.SeriesLabelsQuery
	series *model.LabelMerger
}

func newSeriesLabelsMerger() reportMerger { return new(seriesLabelsMerger) }

func (m *seriesLabelsMerger) merge(report *querybackendv1.Report) error {
	r := report.SeriesLabels
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.series = model.NewLabelMerger()
	})
	m.series.MergeSeries(r.SeriesLabels)
	return nil
}

func (m *seriesLabelsMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query:        m.query,
			SeriesLabels: m.series.SeriesLabels(),
		},
	}
}
