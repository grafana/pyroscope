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
		queryTree,
		newTreeMerger,
		[]section{
			sectionTSDB,
			sectionProfiles,
			sectionSymbols,
		}...,
	)
}

func queryTree(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
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

func newTreeMerger() reportMerger { return new(treeMerger) }

func (m *treeMerger) merge(report *querybackendv1.Report) error {
	r := report.Tree
	m.init.Do(func() {
		m.tree = model.NewTreeMerger()
		m.query = r.Query.CloneVT()
	})
	return m.tree.MergeTreeBytes(r.Tree)
}

func (m *treeMerger) report() *querybackendv1.Report {
	return &querybackendv1.Report{
		Tree: &querybackendv1.TreeReport{
			Query: m.query,
			Tree:  m.tree.Tree().Bytes(m.query.GetMaxNodes()),
		},
	}
}
