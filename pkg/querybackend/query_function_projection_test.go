package querybackend

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (s *testSuite) Test_QueryFunctionProjection() {
	invoke := func(query *queryv1.Query) *queryv1.Report {
		resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
			EndTime:       time.Now().UnixMilli(),
			LabelSelector: "{}",
			QueryPlan:     s.plan,
			Tenant:        s.tenant,
			Query:         []*queryv1.Query{query},
		})
		s.Require().NoError(err)
		s.Require().Len(resp.Reports, 1)
		return resp.Reports[0]
	}
	full, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](invoke(&queryv1.Query{
		QueryType: queryv1.QueryType_QUERY_TREE,
		Tree:      &queryv1.TreeQuery{},
	}).Tree.Tree)
	s.Require().NoError(err)
	var stacks [][]string
	var values []int64
	full.IterateStacks(func(_ model.FunctionName, self int64, stack []model.FunctionName) {
		names := make([]string, len(stack))
		for i, name := range stack {
			names[i] = string(name)
		}
		slices.Reverse(names)
		// UnmarshalTree retains the serialized virtual root in parent pointers.
		if len(names) > 0 && names[0] == "" {
			names = names[1:]
		}
		stacks = append(stacks, names)
		values = append(values, self)
	})
	s.Require().NotEmpty(stacks)
	var path []string
	for _, stack := range stacks {
		if len(stack) > len(path) {
			path = stack
		}
	}
	s.Require().Greater(len(path), 1)
	for _, prefix := range [][]string{nil, path[:2], {"missing"}} {
		selector := &typesv1.StackTraceSelector{}
		for _, name := range prefix {
			selector.CallSite = append(selector.CallSite, &typesv1.Location{Name: name})
		}
		got := invoke(&queryv1.Query{
			QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
			Functions: &queryv1.FunctionsQuery{StackTraceSelector: selector},
		}).Functions.Table
		want := make(map[string]*typesv1.FunctionStats)
		var total int64
		for i, stack := range stacks {
			if values[i] <= 0 || len(stack) < len(prefix) || !slices.Equal(stack[:len(prefix)], prefix) {
				continue
			}
			total += values[i]
			seen := make(map[string]bool)
			for j, name := range stack {
				f := want[name]
				if f == nil {
					f = &typesv1.FunctionStats{Name: name}
					want[name] = f
				}
				if !seen[name] {
					f.Total += values[i]
					seen[name] = true
				}
				if j == len(stack)-1 {
					f.Self += values[i]
				}
			}
		}
		s.Equal(total, got.Total)
		s.Equal(int64(len(want)), got.TotalFunctions)
		for _, f := range got.Functions {
			s.True(want[f.Name].EqualVT(f))
		}
	}
}

func (s *testSuite) Test_QueryFunctions_SlowFixture() {
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: `{service_name="test-app",function="slow"}`,
		QueryPlan:     s.plan,
		Tenant:        s.tenant,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
			Functions: &queryv1.FunctionsQuery{},
		}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	got := resp.Reports[0].Functions.Table
	s.Equal(int64(2985820298582), got.Total)
	s.Equal(int64(10), got.TotalFunctions)
	s.Len(got.Functions, 10)
	byName := make(map[string]*typesv1.FunctionStats)
	for _, f := range got.Functions {
		byName[f.Name] = f
	}
	// The fixed slow fixture has two runtime/pprof.Do frames per stack;
	// inclusive function totals count each sample once, while self stays at the leaf.
	for _, want := range []*typesv1.FunctionStats{
		{
			Name:  "runtime/pprof.Do",
			Total: 2985820298582,
		},
		{
			Name:  "main.work",
			Total: 2985820298582,
			Self:  2875070287507,
		},
		{
			Name:  "runtime.asyncPreempt",
			Total: 110750011075,
			Self:  110750011075,
		},
	} {
		s.True(want.EqualVT(byName[want.Name]), "function %s: want %v, got %v", want.Name, want, byName[want.Name])
	}
}

func TestProjectionQueryRegistration(t *testing.T) {
	for query, report := range map[queryv1.QueryType]queryv1.ReportType{
		queryv1.QueryType_QUERY_FUNCTIONS: queryv1.ReportType_REPORT_FUNCTIONS,
	} {
		require.Equal(t, report, QueryReportType(query))
		_, err := getQueryHandler(query)
		require.NoError(t, err)
	}
	_, err := getQueryHandler(queryv1.QueryType(999))
	require.ErrorContains(t, err, "unknown query type")
}

func TestProjectionQueriesRejectGoPGO(t *testing.T) {
	selector := &typesv1.StackTraceSelector{GoPgo: &typesv1.GoPGO{}}
	for _, query := range []*queryv1.Query{
		{
			QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
			Functions: &queryv1.FunctionsQuery{StackTraceSelector: selector},
		},
	} {
		t.Run(query.QueryType.String(), func(t *testing.T) {
			handler, err := getQueryHandler(query.QueryType)
			require.NoError(t, err)
			report, err := handler(nil, query)
			require.EqualError(t, err, "function projections do not support go_pgo")
			require.Nil(t, report)
		})
	}
}
