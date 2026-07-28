package querybackend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// Test_QueryTree_FullSymbols_Basic verifies that a tree query with FullSymbols=true
// returns a populated symbol table whose slice lengths are consistent.
func (s *testSuite) Test_QueryTree_FullSymbols_Basic() {
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)

	report := resp.Reports[0].Tree
	sym := report.Symbols
	s.Require().NotNil(sym, "Symbols must be non-nil when FullSymbols=true")

	// Index 0 is the sentinel; we must have real entries beyond it.
	s.Assert().Greater(len(sym.Strings), 1)
	s.Assert().Len(sym.StringHashes, len(sym.Strings))
	s.Assert().Greater(len(sym.Locations), 1)
	s.Assert().Len(sym.LocationHashes, len(sym.Locations))
	s.Assert().Greater(len(sym.Functions), 1)
	s.Assert().Len(sym.FunctionHashes, len(sym.Functions))

	tree, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](report.Tree)
	s.Require().NoError(err)
	s.Assert().Greater(tree.Total(), int64(0))
}

// Test_QueryTree_FullSymbols_NotSetByDefault ensures that the Symbols field is nil
// when FullSymbols is not requested.
func (s *testSuite) Test_QueryTree_FullSymbols_NotSetByDefault() {
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Nil(resp.Reports[0].Tree.Symbols)
}

// Test_QueryTree_FullSymbols_TotalsMatchNonFullSymbols verifies that the full-symbols
// path (LocationRefName tree) and the standard path (FuntionName tree) produce the
// same total sample count for identical queries, since both resolve the same samples.
func (s *testSuite) Test_QueryTree_FullSymbols_TotalsMatchNonFullSymbols() {
	invoke := func(mode queryv1.SymbolMode) *queryv1.TreeReport {
		resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
			EndTime:       time.Now().UnixMilli(),
			LabelSelector: "{}",
			QueryPlan:     s.plan,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree:      &queryv1.TreeQuery{SymbolMode: mode},
			}},
			Tenant: s.tenant,
		})
		s.Require().NoError(err)
		s.Require().Len(resp.Reports, 1)
		return resp.Reports[0].Tree
	}

	lrTree, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](invoke(queryv1.SymbolMode_SYMBOL_MODE_FULL).Tree)
	s.Require().NoError(err)
	fnTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](invoke(queryv1.SymbolMode_SYMBOL_MODE_NAME).Tree)
	s.Require().NoError(err)

	s.Assert().Equal(fnTree.Total(), lrTree.Total())
}

// Test_QueryTree_FullSymbols_SymbolConsistency verifies that every location ID
// referenced in the serialised tree is a valid index into Symbols.Locations, and
// that every function ID within those locations is valid in Symbols.Functions.
func (s *testSuite) Test_QueryTree_FullSymbols_SymbolConsistency() {
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)

	report := resp.Reports[0].Tree
	sym := report.Symbols
	nLocations := len(sym.Locations)
	nFunctions := len(sym.Functions)

	tree, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](report.Tree)
	s.Require().NoError(err)

	tree.IterateStacks(func(_ phlaremodel.LocationRefName, _ int64, stack []phlaremodel.LocationRefName) {
		for _, locID := range stack {
			if locID == phlaremodel.OtherLocationRef || locID == 0 {
				continue
			}
			idx := int(locID)
			s.Require().Less(idx, nLocations, "location ID %d out of bounds (have %d locations)", idx, nLocations)
			for _, line := range sym.Locations[idx].Line {
				s.Require().Less(int(line.FunctionId), nFunctions, "function ID %d out of bounds (have %d functions)", line.FunctionId, nFunctions)
			}
		}
	})
}

// Test_QueryTree_FullSymbols_NoDuplicateStrings verifies that the SymbolMerger
// deduplicates the string table correctly when merging results from multiple blocks:
// each unique string must appear exactly once.
func (s *testSuite) Test_QueryTree_FullSymbols_NoDuplicateStrings() {
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)

	sym := resp.Reports[0].Tree.Symbols
	seen := make(map[string]struct{}, len(sym.Strings))
	for _, str := range sym.Strings {
		_, already := seen[str]
		s.Assert().False(already, "duplicate string %q in merged symbol table", str)
		seen[str] = struct{}{}
	}
}

// Test_QueryTree_FullSymbols_Filter verifies that a label-selector filter produces
// a smaller total sample count and a smaller symbol table than an unfiltered query.
func (s *testSuite) Test_QueryTree_FullSymbols_Filter() {
	invoke := func(selector string) *queryv1.TreeReport {
		resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
			EndTime:       time.Now().UnixMilli(),
			LabelSelector: selector,
			QueryPlan:     s.plan,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree:      &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL},
			}},
			Tenant: s.tenant,
		})
		s.Require().NoError(err)
		s.Require().Len(resp.Reports, 1)
		return resp.Reports[0].Tree
	}

	all := invoke("{}")
	filtered := invoke(`{service_name="test-app",function="slow"}`)

	allTree, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](all.Tree)
	s.Require().NoError(err)
	filteredTree, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](filtered.Tree)
	s.Require().NoError(err)

	s.Assert().Greater(allTree.Total(), filteredTree.Total())
	s.Assert().Less(len(filtered.Symbols.Locations), len(all.Symbols.Locations))
}

func TestTreeSymbolMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		query   *queryv1.TreeQuery
		want    queryv1.SymbolMode
		wantErr string
	}{
		{name: "unset defaults to name", query: &queryv1.TreeQuery{}, want: queryv1.SymbolMode_SYMBOL_MODE_NAME},
		{name: "name", query: &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_NAME}, want: queryv1.SymbolMode_SYMBOL_MODE_NAME},
		{name: "full", query: &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL}, want: queryv1.SymbolMode_SYMBOL_MODE_FULL},
		{name: "refs", query: &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_REFS}, want: queryv1.SymbolMode_SYMBOL_MODE_REFS},
		{
			name:  "deprecated full_symbols maps to full",
			query: &queryv1.TreeQuery{FullSymbols: true}, //nolint:staticcheck // exercises the deprecated bridge
			want:  queryv1.SymbolMode_SYMBOL_MODE_FULL,
		},
		{
			name:    "full_symbols combined with symbol_mode is rejected",
			query:   &queryv1.TreeQuery{FullSymbols: true, SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_FULL}, //nolint:staticcheck // exercises the deprecated bridge
			wantErr: "must not be combined",
		},
		{
			name:    "unknown mode is rejected",
			query:   &queryv1.TreeQuery{SymbolMode: queryv1.SymbolMode(99)},
			wantErr: "unsupported symbol_mode",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := treeSymbolMode(tc.query)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, mode)
		})
	}
}

// Test_QueryTree_SymbolModeConflict verifies that a query combining the
// deprecated full_symbols bool with an explicit symbol_mode is rejected end
// to end, rather than silently preferring one and dropping the other.
func (s *testSuite) Test_QueryTree_SymbolModeConflict() {
	_, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{FullSymbols: true, SymbolMode: queryv1.SymbolMode_SYMBOL_MODE_REFS}, //nolint:staticcheck // exercises the deprecated bridge
		}},
		Tenant: s.tenant,
	})
	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "full_symbols must not be combined with symbol_mode")
}

// Test_QueryTree_SymbolRefs_NativeDatasetKeepsPlainPath verifies that a
// SYMBOL_MODE_REFS query against a dataset not labeled unsymbolized (every
// dataset in the test fixtures) keeps today's FunctionName path exactly:
// no SymbolRefTable is attached, and the tree matches a names-only query
// byte for byte in structure (same totals, no TREE->PPROF detour).
func (s *testSuite) Test_QueryTree_SymbolRefs_NativeDatasetKeepsPlainPath() {
	invoke := func(mode queryv1.SymbolMode) *queryv1.TreeReport {
		resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
			EndTime:       time.Now().UnixMilli(),
			LabelSelector: "{}",
			QueryPlan:     s.plan,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree:      &queryv1.TreeQuery{MaxNodes: 16, SymbolMode: mode},
			}},
			Tenant: s.tenant,
		})
		s.Require().NoError(err)
		s.Require().Len(resp.Reports, 1)
		return resp.Reports[0].Tree
	}

	plain := invoke(queryv1.SymbolMode_SYMBOL_MODE_NAME)
	symbolRefs := invoke(queryv1.SymbolMode_SYMBOL_MODE_REFS)
	s.Assert().Nil(symbolRefs.SymbolRefs, "a native dataset must not attach a SymbolRefTable")

	plainTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](plain.Tree)
	s.Require().NoError(err)
	symbolRefsTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](symbolRefs.Tree)
	s.Require().NoError(err)
	s.Assert().Equal(plainTree.String(), symbolRefsTree.String())
}
