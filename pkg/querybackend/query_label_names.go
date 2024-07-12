package querybackend

import (
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_LABEL_NAMES,
		querybackendv1.ReportType_REPORT_LABEL_NAMES,
		func(q *queryContext) queryHandler { return q.queryLabelNames },
		func() reportMerger { return new(labelNameMerger) },
	)
}

func (q *queryContext) queryLabelNames(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		LabelNames: &querybackendv1.LabelNamesReport{
			Query:      query.LabelNames.CloneVT(),
			LabelNames: []string{},
		},
	}
	return resp, nil
}

type labelNameMerger struct {
	init  sync.Once
	query *querybackendv1.LabelValuesQuery
	names *model.LabelMerger
}

func (m *labelNameMerger) merge(report *querybackendv1.Report) error {
	r := report.LabelValues
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.names = model.NewLabelMerger()
	})
	m.names.MergeLabelNames(r.LabelValues)
	return nil
}

func (m *labelNameMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.names == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: querybackendv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &querybackendv1.LabelNamesReport{
			LabelNames: m.names.LabelValues(),
		},
	})
}
