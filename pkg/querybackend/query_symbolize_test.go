package querybackend

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/dskit/tenant"
	"github.com/stretchr/testify/assert"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

const fakeSymbolizedFunction = "symbolized-by-fake"

// fakeSymbolizer records its calls and rewrites every location to a single
// synthetic function, making symbolized output easy to recognize in reports.
type fakeSymbolizer struct {
	mu      sync.Mutex
	tenants []string
	err     error
}

func (f *fakeSymbolizer) SymbolizePprof(ctx context.Context, p *profilev1.Profile) error {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.tenants = append(f.tenants, tenantID)
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	nameIdx := int64(len(p.StringTable))
	p.StringTable = append(p.StringTable, fakeSymbolizedFunction)
	fnID := uint64(len(p.Function) + 1)
	p.Function = append(p.Function, &profilev1.Function{Id: fnID, Name: nameIdx})
	for _, loc := range p.Location {
		loc.Line = []*profilev1.Line{{FunctionId: fnID, Line: 1}}
	}
	for _, m := range p.Mapping {
		m.HasFunctions = true
	}
	return nil
}

func (f *fakeSymbolizer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tenants)
}

type limitsFunc func(tenantID string) bool

func (f limitsFunc) SymbolizerEnabled(tenantID string) bool { return f(tenantID) }

func allTenantsEnabled() limitsFunc { return func(string) bool { return true } }

// markFormat0DatasetsUnsymbolized labels every explicit (Format0) dataset of
// the planned blocks as unsymbolized and rebuilds the query plan.
func (s *testSuite) markFormat0DatasetsUnsymbolized() int {
	var marked int
	for _, md := range s.meta {
		n := int32(len(md.StringTable))
		md.StringTable = append(md.StringTable, metadata.LabelNameUnsymbolized, "true")
		for _, ds := range md.Datasets {
			if block.DatasetFormat(ds.Format) != block.DatasetFormat0 {
				continue
			}
			ds.Labels = append(ds.Labels, 1, n, n+1)
			marked++
		}
	}
	s.plan = queryplan.Build(s.meta, 10, 10)
	s.Require().NotZero(marked)
	return marked
}

// markFirstFormat0BlockUnsymbolized labels all datasets of the first block
// with explicit (Format0) datasets, leaving the remaining blocks untouched.
func (s *testSuite) markFirstFormat0BlockUnsymbolized() (*metastorev1.BlockMeta, int) {
	for _, md := range s.meta {
		var isFormat0 bool
		for _, ds := range md.Datasets {
			if block.DatasetFormat(ds.Format) == block.DatasetFormat0 {
				isFormat0 = true
				break
			}
		}
		if !isFormat0 {
			continue
		}
		n := int32(len(md.StringTable))
		md.StringTable = append(md.StringTable, metadata.LabelNameUnsymbolized, "true")
		var marked int
		for _, ds := range md.Datasets {
			ds.Labels = append(ds.Labels, 1, n, n+1)
			marked++
		}
		return md, marked
	}
	return nil, 0
}

func (s *testSuite) newSymbolizingReader(sym Symbolizer, limits Limits) *BlockReader {
	return NewBlockReader(s.logger, &objstore.ReaderAtBucket{Bucket: s.bucket}, nil, sym, limits)
}

func (s *testSuite) invokeTree(reader *BlockReader, symbolize bool) (*queryv1.InvokeResponse, error) {
	return reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Options:       &queryv1.InvokeOptions{Symbolize: symbolize},
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      new(queryv1.TreeQuery),
		}},
		Tenant: s.tenant,
	})
}

func (s *testSuite) Test_Symbolize_Tree_UnsymbolizedDatasets() {
	marked := s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := s.invokeTree(reader, true)
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)

	s.Assert().Equal(marked, fake.calls())
	tenants := make(map[string]struct{}, len(s.tenant))
	for _, t := range s.tenant {
		tenants[t] = struct{}{}
	}
	for _, t := range fake.tenants {
		_, ok := tenants[t]
		s.Assert().True(ok, "symbolizer must be called with the dataset's owning tenant, got %q", t)
	}

	tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
	s.Require().NoError(err)
	s.Assert().Contains(tree.String(), fakeSymbolizedFunction)
}

func (s *testSuite) Test_Symbolize_Tree_MergesWithNativeDatasets() {
	// Mark the datasets of a single block only: the merged tree must contain
	// both symbolized stacks and stacks from the untouched datasets.
	_, marked := s.markFirstFormat0BlockUnsymbolized()
	s.Require().NotZero(marked)
	s.plan = queryplan.Build(s.meta, 10, 10)

	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := s.invokeTree(reader, true)
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Equal(marked, fake.calls())

	tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
	s.Require().NoError(err)
	rendered := tree.String()
	s.Assert().Contains(rendered, fakeSymbolizedFunction, "symbolized stacks must be in the merged tree")
	s.Assert().Greater(strings.Count(rendered, "\n"), 1, "native stacks must survive the merge")
}

func (s *testSuite) Test_Symbolize_Tree_NotRequested() {
	s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := s.invokeTree(reader, false)
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Zero(fake.calls(), "symbolizer must not be called when the request does not ask for it")
}

func (s *testSuite) Test_Symbolize_Tree_TenantDisabled() {
	s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, limitsFunc(func(string) bool { return false }))

	resp, err := s.invokeTree(reader, true)
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Zero(fake.calls(), "symbolizer must not be called for disabled tenants")
}

func (s *testSuite) Test_Symbolize_Tree_SymbolizedDatasets() {
	// No dataset carries the unsymbolized label.
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := s.invokeTree(reader, true)
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Zero(fake.calls(), "symbolizer must not be called for symbolized datasets")
}

func (s *testSuite) Test_Symbolize_Tree_FullSymbols() {
	s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Options:       &queryv1.InvokeOptions{Symbolize: true},
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{FullSymbols: true},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Zero(fake.calls(), "full-symbols trees are symbolized by the frontend, not the backend")
	s.Assert().NotNil(resp.Reports[0].Tree.Symbols)
}

func (s *testSuite) Test_Symbolize_Tree_SpanSelector() {
	// The span selector is applied before the output is rendered, so the
	// detour must still symbolize span-filtered trees.
	marked := s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Options:       &queryv1.InvokeOptions{Symbolize: true},
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{SpanSelector: []string{"00000000badc0ffe"}},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Equal(marked, fake.calls())
}

func (s *testSuite) Test_Symbolize_Pprof() {
	marked := s.markFormat0DatasetsUnsymbolized()
	fake := &fakeSymbolizer{}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	resp, err := reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: `{service_name="test-app",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`,
		QueryPlan:     s.plan,
		Options:       &queryv1.InvokeOptions{Symbolize: true},
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Equal(marked, fake.calls())

	var p profilev1.Profile
	s.Require().NoError(pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &p))
	var found bool
	for _, str := range p.StringTable {
		if str == fakeSymbolizedFunction {
			found = true
			break
		}
	}
	s.Assert().True(found, "merged profile must contain the symbolized function")
}

func (s *testSuite) Test_Symbolize_ErrorPropagates() {
	// The plan is restricted to a single block: a failing handler cancels
	// the whole request, and sibling blocks would be torn down mid-read.
	md, marked := s.markFirstFormat0BlockUnsymbolized()
	s.Require().NotZero(marked)
	s.plan = queryplan.Build([]*metastorev1.BlockMeta{md}, 10, 10)

	fake := &fakeSymbolizer{err: errors.New("debuginfod is on fire")}
	reader := s.newSymbolizingReader(fake, allTenantsEnabled())

	_, err := s.invokeTree(reader, true)
	s.Require().ErrorContains(err, "debuginfod is on fire")
}

// BenchmarkQueryTree_BackendSymbolization measures the cost of the pprof
// detour itself: the symbolizer is a fake, so symbol resolution is excluded.
func BenchmarkQueryTree_BackendSymbolization(b *testing.B) {
	for _, mode := range []struct {
		name      string
		symbolize bool
	}{
		{name: "native", symbolize: false},
		{name: "symbolize", symbolize: true},
	} {
		b.Run(mode.name, func(b *testing.B) {
			f := setupBenchmarkFixture(b)
			for _, md := range f.plan.Root.Blocks {
				n := int32(len(md.StringTable))
				md.StringTable = append(md.StringTable, metadata.LabelNameUnsymbolized, "true")
				for _, ds := range md.Datasets {
					if block.DatasetFormat(ds.Format) == block.DatasetFormat0 {
						ds.Labels = append(ds.Labels, 1, n, n+1)
					}
				}
			}
			reader := NewBlockReader(test.NewTestingLogger(b), &objstore.ReaderAtBucket{Bucket: f.bucket}, nil,
				&fakeSymbolizer{}, allTenantsEnabled())
			req := &queryv1.InvokeRequest{
				EndTime:       time.Now().UnixMilli(),
				LabelSelector: "{}",
				QueryPlan:     f.plan,
				Options:       &queryv1.InvokeOptions{Symbolize: mode.symbolize},
				Query: []*queryv1.Query{{
					QueryType: queryv1.QueryType_QUERY_TREE,
					Tree:      &queryv1.TreeQuery{MaxNodes: 16},
				}},
				Tenant: f.tenant,
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := reader.Invoke(f.ctx, req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func Test_datasetUnsymbolized(t *testing.T) {
	st := []string{"", "service_name", "test-app", metadata.LabelNameUnsymbolized, "true", "false"}
	md := &metastorev1.BlockMeta{StringTable: st}

	for _, tc := range []struct {
		name   string
		labels []int32
		want   bool
	}{
		{name: "no labels", labels: nil, want: false},
		{name: "no unsymbolized label", labels: []int32{1, 1, 2}, want: false},
		{name: "unsymbolized", labels: []int32{2, 1, 2, 3, 4}, want: true},
		{name: "unsymbolized false value", labels: []int32{2, 1, 2, 3, 5}, want: false},
		// Compacted datasets accumulate label sets of their sources:
		// any single set carrying the label is sufficient.
		{name: "compacted multi-set", labels: []int32{1, 1, 2, 2, 1, 2, 3, 4}, want: true},
		{name: "compacted multi-set unlabeled", labels: []int32{1, 1, 2, 1, 1, 2}, want: false},
		// Malformed metadata must not panic nor match.
		{name: "out of range indices", labels: []int32{1, 42, 43}, want: false},
		{name: "negative indices", labels: []int32{1, -1, -2}, want: false},
		{name: "truncated set", labels: []int32{3, 3, 4}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, datasetUnsymbolized(md, &metastorev1.Dataset{Labels: tc.labels}))
		})
	}
}
