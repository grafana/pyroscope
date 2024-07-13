package querybackend

import (
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_LABEL_VALUES,
		querybackendv1.ReportType_REPORT_LABEL_VALUES,
		queryLabelValues,
		newLabelValueMerger,
		[]section{sectionTSDB}...,
	)
}

func queryLabelValues(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	values, err := q.svc.tsdb.LabelValues(query.LabelValues.LabelName, q.req.matchers...)
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
