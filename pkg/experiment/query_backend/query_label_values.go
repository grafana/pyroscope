package query_backend

import (
	"errors"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_LABEL_VALUES,
		queryv1.ReportType_REPORT_LABEL_VALUES,
		queryLabelValues,
		newLabelValueAggregator,
		[]block.Section{block.SectionTSDB}...,
	)
}

func queryLabelValues(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	var values []string
	var err error
	if len(q.req.matchers) == 0 {
		values, err = q.ds.Index().LabelValues(query.LabelValues.LabelName)
	} else {
		values, err = labelValuesForMatchers(q.ds.Index(), query.LabelValues.LabelName, q.req.matchers)
	}
	if err != nil {
		return nil, err
	}
	resp := &queryv1.Report{
		LabelValues: &queryv1.LabelValuesReport{
			Query:       query.LabelValues.CloneVT(),
			LabelValues: values,
		},
	}
	return resp, nil
}

func labelValuesForMatchers(reader phlaredb.IndexReader, name string, matchers []*labels.Matcher) ([]string, error) {
	postings, err := phlaredb.PostingsForMatchers(reader, nil, matchers...)
	if err != nil {
		return nil, err
	}
	l := make(map[string]struct{})
	for postings.Next() {
		var v string
		if v, err = reader.LabelValueFor(postings.At(), name); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, err
		}
		l[v] = struct{}{}
	}
	if err = postings.Err(); err != nil {
		return nil, err
	}
	values := make([]string, len(l))
	var i int
	for v := range l {
		values[i] = v
		i++
	}
	sort.Strings(values)
	return values, nil
}

type labelValueAggregator struct {
	init   sync.Once
	query  *queryv1.LabelValuesQuery
	values *model.LabelMerger
}

func newLabelValueAggregator(*queryv1.InvokeRequest) aggregator {
	return new(labelValueAggregator)
}

func (m *labelValueAggregator) aggregate(report *queryv1.Report) error {
	r := report.LabelValues
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.values = model.NewLabelMerger()
	})
	m.values.MergeLabelValues(r.LabelValues)
	return nil
}

func (m *labelValueAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		LabelValues: &queryv1.LabelValuesReport{
			Query:       m.query,
			LabelValues: m.values.LabelValues(),
		},
	}
}
