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
		queryLabelNames,
		newLabelNameMerger,
		[]section{sectionTSDB}...,
	)
}

func queryLabelNames(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	names, err := q.svc.tsdb.LabelNames(q.req.matchers...)
	if err != nil {
		return nil, err
	}
	resp := &querybackendv1.Report{
		LabelNames: &querybackendv1.LabelNamesReport{
			Query:      query.LabelNames.CloneVT(),
			LabelNames: names,
		},
	}
	return resp, nil
}

type labelNameMerger struct {
	init  sync.Once
	query *querybackendv1.LabelValuesQuery
	names *model.LabelMerger
}

func newLabelNameMerger() reportMerger { return new(labelNameMerger) }

func (m *labelNameMerger) merge(report *querybackendv1.Report) error {
	r := report.LabelValues
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.names = model.NewLabelMerger()
	})
	m.names.MergeLabelNames(r.LabelValues)
	return nil
}

func (m *labelNameMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		LabelNames: &querybackendv1.LabelNamesReport{
			LabelNames: m.names.LabelValues(),
		},
	}
}
