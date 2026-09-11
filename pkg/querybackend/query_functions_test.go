package querybackend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

func (s *testSuite) Test_QueryFunctions() {
	// The same data is queried in two formats within one plan, exercising the
	// real block readers and multi-dataset report aggregation.
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime: time.Now().UnixMilli(), LabelSelector: "{}", QueryPlan: s.plan, Tenant: s.tenant,
		Query: []*queryv1.Query{
			{QueryType: queryv1.QueryType_QUERY_FUNCTIONS, Functions: &queryv1.FunctionsQuery{}},
			{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{}},
		},
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 2)
	var table *querierv1.FunctionTable
	var tree *model.FunctionNameTree
	for _, report := range resp.Reports {
		switch report.ReportType {
		case queryv1.ReportType_REPORT_FUNCTIONS:
			table = report.Functions.Functions
		case queryv1.ReportType_REPORT_TREE:
			tree, err = model.UnmarshalTree[model.FunctionName, model.FunctionNameI](report.Tree.Tree)
			s.Require().NoError(err)
		}
	}
	s.Require().NotNil(table)
	s.Require().NotNil(tree)
	want, err := model.FunctionTableFromTree(s.ctx, tree)
	s.Require().NoError(err)
	model.LimitFunctionTable(want, -1)
	model.LimitFunctionTable(table, -1)
	s.Require().Greater(table.Total, int64(0))
	s.Require().Greater(len(table.Functions), 16)
	s.Require().Equal(want, table)
}

func TestFunctionsAggregator_RetainsAllRows(t *testing.T) {
	t.Parallel()
	a := newFunctionsAggregator(nil)
	for _, name := range []string{"a", "b"} {
		require.NoError(t, a.aggregate(&queryv1.Report{Functions: &queryv1.FunctionsReport{
			Query: &queryv1.FunctionsQuery{},
			Functions: &querierv1.FunctionTable{Total: 19, Functions: []*querierv1.FunctionRow{
				{Name: name, Self: 10, Total: 10}, {Name: "global", Self: 9, Total: 9},
			}},
		}}))
	}
	table := a.build().Functions.Functions
	require.Len(t, table.Functions, 3)
	model.LimitFunctionTable(table, 1)
	require.Equal(t, int64(38), table.Total)
	require.Equal(t, []*querierv1.FunctionRow{{Name: "global", Self: 18, Total: 18}}, table.Functions)
}

func (s *testSuite) Test_QueryFunctions_Selectors() {
	var baseline int64
	for _, tc := range []struct {
		name      string
		query     *queryv1.FunctionsQuery
		wantErr   bool
		wantEmpty bool
	}{
		{name: "all", query: &queryv1.FunctionsQuery{}},
		{name: "profile", query: &queryv1.FunctionsQuery{ProfileIdSelector: []string{s.getProfileIDFromExemplars(s.T())}}},
		{name: "span", query: &queryv1.FunctionsQuery{SpanSelector: []string{fixtureMatchingSpanID}}},
		{name: "trace", query: &queryv1.FunctionsQuery{TraceIdSelector: []string{fixtureMatchingTraceID}}},
		{name: "absent span", query: &queryv1.FunctionsQuery{SpanSelector: []string{fixtureNonMatchingSpanID}}, wantEmpty: true},
		{name: "absent trace", query: &queryv1.FunctionsQuery{TraceIdSelector: []string{fixtureNonMatchingTraceID}}, wantEmpty: true},
		{name: "invalid profile", query: &queryv1.FunctionsQuery{ProfileIdSelector: []string{"invalid"}}, wantErr: true},
		{name: "invalid span", query: &queryv1.FunctionsQuery{SpanSelector: []string{"invalid"}}, wantErr: true},
		{name: "invalid trace", query: &queryv1.FunctionsQuery{TraceIdSelector: []string{"invalid"}}, wantErr: true},
		{name: "span and trace", query: &queryv1.FunctionsQuery{SpanSelector: []string{fixtureMatchingSpanID}, TraceIdSelector: []string{fixtureMatchingTraceID}}, wantErr: true},
	} {
		s.Run(tc.name, func() {
			resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				EndTime: time.Now().UnixMilli(), LabelSelector: "{}", QueryPlan: s.plan, Tenant: s.tenant,
				Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_FUNCTIONS, Functions: tc.query}},
			})
			if tc.wantErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Require().Len(resp.Reports, 1)
			table := resp.Reports[0].Functions.Functions
			if tc.wantEmpty {
				s.Require().Zero(table.Total)
				s.Require().Empty(table.Functions)
				return
			}
			s.Require().Positive(table.Total)
			if tc.name == "all" {
				baseline = table.Total
			} else {
				s.Require().Less(table.Total, baseline)
			}
		})
	}
}
