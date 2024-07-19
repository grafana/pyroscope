package querybackend

import (
	"errors"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/querybackend/block"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_LABEL_VALUES,
		querybackendv1.ReportType_REPORT_LABEL_VALUES,
		queryLabelValues,
		newLabelValueMerger,
		[]block.Section{block.SectionTSDB}...,
	)
}

func queryLabelValues(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	var values []string
	var err error
	if len(q.req.matchers) == 0 {
		values, err = q.svc.Index().LabelValues(query.LabelValues.LabelName)
	} else {
		values, err = labelValuesForMatchers(q.svc.Index(), query.LabelValues.LabelName, q.req.matchers)
	}
	if err != nil {
		return nil, err
	}
	resp := &querybackendv1.Report{
		LabelValues: &querybackendv1.LabelValuesReport{
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

type labelValueMerger struct {
	init   sync.Once
	query  *querybackendv1.LabelValuesQuery
	values *model.LabelMerger
}

func newLabelValueMerger() reportMerger { return new(labelValueMerger) }

func (m *labelValueMerger) merge(report *querybackendv1.Report) error {
	r := report.LabelValues
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.values = model.NewLabelMerger()
	})
	m.values.MergeLabelValues(r.LabelValues)
	return nil
}

func (m *labelValueMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		LabelValues: &querybackendv1.LabelValuesReport{
			Query:       m.query,
			LabelValues: m.values.LabelValues(),
		},
	}
}
