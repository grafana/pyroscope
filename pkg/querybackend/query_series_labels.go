package querybackend

import (
	"errors"
	"slices"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_SERIES_LABELS,
		queryv1.ReportType_REPORT_SERIES_LABELS,
		querySeriesLabels,
		newSeriesLabelsAggregator,
		false,
		[]block.Section{block.SectionTSDB}...,
	)
}

func querySeriesLabels(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	series, err := getSeriesLabels(q.ds.Index(), q.req.matchers, query.SeriesLabels.LabelNames...)
	if err != nil {
		return nil, err
	}
	resp := &queryv1.Report{
		SeriesLabels: &queryv1.SeriesLabelsReport{
			Query:        query.SeriesLabels.CloneVT(),
			SeriesLabels: series,
		},
	}
	return resp, nil
}

type seriesLabelsAggregator struct {
	init   sync.Once
	query  *queryv1.SeriesLabelsQuery
	series *phlaremodel.LabelMerger
}

func newSeriesLabelsAggregator(*queryv1.InvokeRequest) aggregator {
	return new(seriesLabelsAggregator)
}

func (a *seriesLabelsAggregator) aggregate(report *queryv1.Report) error {
	r := report.SeriesLabels
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
		a.series = phlaremodel.NewLabelMerger()
	})
	a.series.MergeSeries(r.SeriesLabels)
	return nil
}

func (a *seriesLabelsAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		SeriesLabels: &queryv1.SeriesLabelsReport{
			Query:        a.query,
			SeriesLabels: a.series.Labels(),
		},
	}
}

func getSeriesLabels(reader phlaredb.IndexReader, matchers []*labels.Matcher, by ...string) ([]*typesv1.Labels, error) {
	names, err := reader.LabelNames()
	if err != nil {
		return nil, err
	}
	if len(by) > 0 {
		names = slices.DeleteFunc(names, func(n string) bool {
			for j := 0; j < len(by); j++ {
				if by[j] == n {
					return false
				}
			}
			return true
		})
		if len(names) == 0 {
			return nil, nil
		}
	}

	postings, err := getPostings(reader, matchers...)
	if err != nil {
		return nil, err
	}

	visited := make(map[uint64]struct{})
	sets := make([]*typesv1.Labels, 0, 32)
	ls := make(phlaremodel.Labels, len(names))
	for i := range names {
		ls[i] = new(typesv1.LabelPair)
	}

	for postings.Next() {
		var j int
		var v string
		for i := range names {
			v, err = reader.LabelValueFor(postings.At(), names[i])
			switch {
			case err == nil:
			case errors.Is(err, storage.ErrNotFound):
			default:
				return nil, err
			}
			if v != "" {
				ls[j].Name = names[i]
				ls[j].Value = v
				j++
			}
		}
		set := ls[:j]
		h := set.Hash()
		if _, ok := visited[h]; !ok {
			visited[h] = struct{}{}
			sets = append(sets, &typesv1.Labels{Labels: set.Clone()})
		}
	}

	return sets, postings.Err()
}
