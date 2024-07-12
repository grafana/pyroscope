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
		func(q *queryContext) queryHandler { return q.queryLabelValues },
		func() reportMerger { return new(labelValueMerger) },
	)
}

func (q *queryContext) queryLabelValues(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		LabelValues: &querybackendv1.LabelValuesReport{
			Query:       query.LabelValues.CloneVT(),
			LabelValues: []string{},
		},
	}
	return resp, nil
}

type labelValueMerger struct {
	init   sync.Once
	query  *querybackendv1.LabelValuesQuery
	values *model.LabelMerger
}

func (m *labelValueMerger) merge(report *querybackendv1.Report) error {
	r := report.LabelValues
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.values = model.NewLabelMerger()
	})
	m.values.MergeLabelValues(r.LabelValues)
	return nil
}

func (m *labelValueMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.values == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: querybackendv1.ReportType_REPORT_LABEL_VALUES,
		LabelValues: &querybackendv1.LabelValuesReport{
			LabelValues: m.values.LabelValues(),
		},
	})
}
