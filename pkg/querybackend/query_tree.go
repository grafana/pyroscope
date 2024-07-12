package querybackend

import (
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_TREE,
		querybackendv1.ReportType_REPORT_TREE,
		func(q *queryContext) queryHandler { return q.queryTree },
		func() reportMerger { return new(treeMerger) },
	)
}

func (q *queryContext) queryTree(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		Tree: &querybackendv1.TreeReport{
			Query: query.Tree.CloneVT(),
			Tree:  new(model.Tree).Bytes(query.Tree.GetMaxNodes()),
		},
	}
	return resp, nil
}

type treeMerger struct {
	init  sync.Once
	query *querybackendv1.TreeQuery
	tree  *model.TreeMerger
}

func (m *treeMerger) merge(report *querybackendv1.Report) error {
	r := report.Tree
	m.init.Do(func() {
		m.tree = model.NewTreeMerger()
		m.query = r.Query.CloneVT()
	})
	return m.tree.MergeTreeBytes(r.Tree)
}

func (m *treeMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.tree == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: querybackendv1.ReportType_REPORT_TREE,
		Tree: &querybackendv1.TreeReport{
			Query: m.query,
			Tree:  m.tree.Tree().Bytes(m.query.GetMaxNodes()),
		},
	})
}
