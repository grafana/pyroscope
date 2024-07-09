package querybackend

import (
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func (q *queryContext) queryTree(query *querybackendv1.TreeQuery) (*querybackendv1.Report, error) {
	// TODO: implement
	resp := &querybackendv1.Report{
		ReportType: &querybackendv1.Report_Tree{
			Tree: &querybackendv1.TreeReport{
				Query: query.CloneVT(),
				Data:  new(model.Tree).Bytes(query.GetMaxNodes()),
			},
		},
	}
	return resp, nil
}
