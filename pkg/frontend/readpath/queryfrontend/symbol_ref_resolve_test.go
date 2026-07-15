package queryfrontend

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/lidia"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

// buildSymbolRefFixture returns a symbol-ref tree/table pair rooted at a
// resolved frame ("known_func") with two unresolved locations on build
// "build-a" (0x100 and 0x200, deliberately out of address order) and one on
// build "build-b" (0x300).
func buildSymbolRefFixture(t *testing.T) ([]byte, *queryv1.SymbolRefTable) {
	t.Helper()
	table := symbolref.NewTable()
	known := table.InternName("known_func")
	locHit := table.InternUnresolved("build-a", "libfoo.so", 0x100)
	locMiss := table.InternUnresolved("build-a", "libfoo.so", 0x200)
	locOther := table.InternUnresolved("build-b", "libbar.so", 0x300)

	tree := new(phlaremodel.LocationRefNameTree)
	tree.InsertStack(1, known)
	tree.InsertStack(2, known, locHit)
	tree.InsertStack(3, known, locMiss)
	tree.InsertStack(4, known, locOther)

	rb := table.ResultBuilder()
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)
	return treeBytes, pb
}

func flameNames(t *testing.T, treeBytes []byte) []string {
	t.Helper()
	tr, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](treeBytes)
	require.NoError(t, err)
	return phlaremodel.NewFlameGraph(tr, -1).Names
}

func TestUseSymbolRefTrees(t *testing.T) {
	tests := []struct {
		name             string
		noSymbolizer     bool
		tenants          []string
		enabledPerTID    map[string]bool
		symbolizerPerTID map[string]bool // SymbolizerEnabled per tenant; defaults to true
		want             bool
	}{
		{
			name:         "no symbolizer configured",
			noSymbolizer: true,
			tenants:      []string{"tenant-a"},
			want:         false,
		},
		{
			name:          "single tenant enabled",
			tenants:       []string{"tenant-a"},
			enabledPerTID: map[string]bool{"tenant-a": true},
			want:          true,
		},
		{
			name:          "single tenant disabled",
			tenants:       []string{"tenant-a"},
			enabledPerTID: map[string]bool{"tenant-a": false},
			want:          false,
		},
		{
			name:          "all tenants enabled",
			tenants:       []string{"tenant-a", "tenant-b"},
			enabledPerTID: map[string]bool{"tenant-a": true, "tenant-b": true},
			want:          true,
		},
		{
			name:          "one of several tenants disabled",
			tenants:       []string{"tenant-a", "tenant-b"},
			enabledPerTID: map[string]bool{"tenant-a": true, "tenant-b": false},
			want:          false,
		},
		{
			name:             "flag on but symbolization disabled for the tenant",
			tenants:          []string{"tenant-a"},
			enabledPerTID:    map[string]bool{"tenant-a": true},
			symbolizerPerTID: map[string]bool{"tenant-a": false},
			want:             false,
		},
		{
			name:             "symbolization disabled for one of several tenants",
			tenants:          []string{"tenant-a", "tenant-b"},
			enabledPerTID:    map[string]bool{"tenant-a": true, "tenant-b": true},
			symbolizerPerTID: map[string]bool{"tenant-a": true, "tenant-b": false},
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			for _, tid := range tt.tenants {
				symbolizerEnabled, ok := tt.symbolizerPerTID[tid]
				if !ok {
					symbolizerEnabled = true
				}
				mockLimits.On("SymbolizerEnabled", tid).Return(symbolizerEnabled).Maybe()
				mockLimits.On("SymbolRefTreesEnabled", tid).Return(tt.enabledPerTID[tid]).Maybe()
			}

			var sym Symbolizer
			if !tt.noSymbolizer {
				sym = mockqueryfrontend.NewMockSymbolizer(t)
			}

			qf := &QueryFrontend{limits: mockLimits, symbolizer: sym}
			require.Equal(t, tt.want, qf.useSymbolRefTrees(tt.tenants))
		})
	}
}

// TestSelectMergeStacktracesTree_SymbolRefFlagOn verifies that once the
// per-tenant flag is enabled, the request sets TreeQuery.SymbolRefs and is
// sent as-is (no backendTreeSymbolizer wrapping, no QUERY_TREE -> QUERY_PPROF
// rewrite), even when block metadata reports unsymbolized profiles -- the
// exact condition that would trigger the legacy detour with the flag off.
func TestSelectMergeStacktracesTree_SymbolRefFlagOn(t *testing.T) {
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesDefault", "tenant1").Return(0)
	mockLimits.On("QuerySanitizeOnMerge", "tenant1").Return(false)
	mockLimits.On("SymbolizerEnabled", "tenant1").Return(true)
	mockLimits.On("SymbolRefTreesEnabled", "tenant1").Return(true)

	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)

	plainTree := new(phlaremodel.FunctionNameTree)
	plainTree.InsertStack(1, phlaremodel.FunctionName("some_func"))

	mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockQueryBackend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
		require.Len(t, req.Query, 1)
		require.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
		require.True(t, req.Query[0].Tree.GetSymbolRefs())
		return true
	})).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_TREE,
			Tree:       &queryv1.TreeReport{Tree: plainTree.Bytes(0, nil)},
		}},
	}, nil).Once()

	mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{
			Id:          "block_id",
			Datasets:    []*metastorev1.Dataset{{Labels: []int32{1, 1, 2}}},
			StringTable: []string{"", "__unsymbolized__", "true"},
		}},
	}, nil).Once()

	qf := NewQueryFrontend(log.NewNopLogger(), mockLimits, mockMetadataClient, nil, mockQueryBackend, mockSymbolizer, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), "tenant1")
	start, end := smpValidTimeRange()
	resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="test-service"}`,
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	require.Contains(t, resp.Msg.GetFlamegraph().GetNames(), "some_func")
	mockQueryBackend.AssertExpectations(t)
	// mockSymbolizer.Resolve must not be called: the response carried no
	// SymbolRefs table (nothing to resolve).
}

// TestSelectMergeStacktracesTree_SymbolRefResolution verifies that a
// response with unresolved symbol-ref locations is resolved through the
// symbolizer, grouped once per distinct build ID with sorted, deduped
// addresses, and rebuilt into a plain tree carrying both resolved names and
// the binary!0xaddr fallback for an address the symbolizer misses.
func TestSelectMergeStacktracesTree_SymbolRefResolution(t *testing.T) {
	treeBytes, pb := buildSymbolRefFixture(t)

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesDefault", "tenant1").Return(0)
	mockLimits.On("QuerySanitizeOnMerge", "tenant1").Return(false)
	mockLimits.On("SymbolizerEnabled", "tenant1").Return(true)
	mockLimits.On("SymbolRefTreesEnabled", "tenant1").Return(true)
	mockLimits.On("SymbolizerResolveTimeout", "tenant1").Return(time.Second)

	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
	mockSymbolizer.On("ResolveConcurrency").Return(4)
	mockSymbolizer.On("Resolve", mock.Anything, "build-a", "libfoo.so", []uint64{0x100, 0x200}).
		Return([][]lidia.SourceInfoFrame{
			{{FunctionName: "resolved_a"}}, // 0x100 hits
			nil,                            // 0x200 misses
		}, nil).Once()
	mockSymbolizer.On("Resolve", mock.Anything, "build-b", "libbar.so", []uint64{0x300}).
		Return([][]lidia.SourceInfoFrame{{{FunctionName: "resolved_b"}}}, nil).Once()

	mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_TREE,
			Tree:       &queryv1.TreeReport{Tree: treeBytes, SymbolRefs: pb},
		}},
	}, nil).Once()

	mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: "block_id"}},
	}, nil).Once()

	qf := NewQueryFrontend(log.NewNopLogger(), mockLimits, mockMetadataClient, nil, mockQueryBackend, mockSymbolizer, nil, nil)

	before := testutil.ToFloat64(qf.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationResolved))

	ctx := tenant.InjectTenantID(context.Background(), "tenant1")
	start, end := smpValidTimeRange()
	resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="test-service"}`,
		Start:         start,
		End:           end,
	}))
	require.NoError(t, err)

	names := resp.Msg.GetFlamegraph().GetNames()
	require.Contains(t, names, "known_func")
	require.Contains(t, names, "resolved_a")
	require.Contains(t, names, "resolved_b")
	require.Contains(t, names, "libfoo.so!0x200")

	require.Equal(t, before+2, testutil.ToFloat64(qf.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationResolved)))
	require.Equal(t, float64(1), testutil.ToFloat64(qf.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationMiss)))

	mockQueryBackend.AssertExpectations(t)
	mockSymbolizer.AssertExpectations(t)
}

// TestBuildLookup_ReversesLidiaFrameOrder verifies the lookup reverses each
// resolved chain from lidia's innermost-first order (pprof Line order) to
// the root-first order Rebuild splices in: without the reversal, inlined
// frames render with parent and child flipped relative to the legacy pprof
// path (see TestRebuildInlineChainExpansionOrder for the Rebuild-side
// contract).
func TestBuildLookup_ReversesLidiaFrameOrder(t *testing.T) {
	qf := NewQueryFrontend(log.NewNopLogger(), nil, nil, nil, nil, nil, nil, nil)
	lookup := qf.buildLookup([]binaryResolution{{
		binary: symbolref.UnresolvedBinary{BuildID: "build-a", BinaryName: "libfoo.so", Addresses: []uint64{0x100}},
		frames: [][]lidia.SourceInfoFrame{{
			{FunctionName: "inner"},
			{FunctionName: "inlined_middle"},
			{FunctionName: "outer"},
		}},
	}})
	require.Equal(t,
		[]symbolref.Frame{{Name: "outer"}, {Name: "inlined_middle"}, {Name: "inner"}},
		lookup.resolve("build-a", 0x100))
}

// TestResolveSymbolRefs_NoSymbolRefTable verifies a report without a
// symbol-ref table (an old backend, or a fully-symbolized dataset) passes
// through completely unchanged: no symbolizer call, tree bytes untouched.
func TestResolveSymbolRefs_NoSymbolRefTable(t *testing.T) {
	qf := &QueryFrontend{metrics: newQueryFrontendMetrics(nil)}
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_TREE,
		Tree:       &queryv1.TreeReport{Tree: []byte("plain-tree-bytes")},
	}

	err := qf.resolveSymbolRefs(context.Background(), []string{"tenant1"}, report, 0)
	require.NoError(t, err)
	require.Equal(t, []byte("plain-tree-bytes"), report.Tree.Tree)
	require.Nil(t, report.Tree.SymbolRefs)
}

// TestResolveSymbolRefs_NoUnresolvedEntries verifies a symbol-ref table with
// zero unresolved entries is left untouched (the degenerate case a
// well-behaved backend collapses to a plain tree report on its own, per I4;
// this only proves the frontend does not error or needlessly rebuild it).
func TestResolveSymbolRefs_NoUnresolvedEntries(t *testing.T) {
	qf := &QueryFrontend{metrics: newQueryFrontendMetrics(nil)}
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{
			Tree:       []byte("ref-space-tree-bytes"),
			SymbolRefs: &queryv1.SymbolRefTable{Names: []string{"a", "b"}},
		},
	}

	err := qf.resolveSymbolRefs(context.Background(), []string{"tenant1"}, report, 0)
	require.NoError(t, err)
	require.Equal(t, []byte("ref-space-tree-bytes"), report.Tree.Tree)
	require.NotNil(t, report.Tree.SymbolRefs)
}

// TestResolveBinaries_PerBinaryTimeoutFallsBack verifies that a binary whose
// own resolve timebox expires while the request context is still live is
// recorded as a miss for its addresses, and the request succeeds.
func TestResolveBinaries_PerBinaryTimeoutFallsBack(t *testing.T) {
	treeBytes, pb := buildSymbolRefFixture(t)

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("SymbolizerResolveTimeout", "tenant1").Return(time.Millisecond)

	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
	mockSymbolizer.On("ResolveConcurrency").Return(4)
	mockSymbolizer.On("Resolve", mock.Anything, "build-a", "libfoo.so", []uint64{0x100, 0x200}).
		Return(func(ctx context.Context, buildID, binaryName string, addrs []uint64) ([][]lidia.SourceInfoFrame, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}).Once()
	mockSymbolizer.On("Resolve", mock.Anything, "build-b", "libbar.so", []uint64{0x300}).
		Return([][]lidia.SourceInfoFrame{{{FunctionName: "resolved_b"}}}, nil).Once()

	qf := &QueryFrontend{limits: mockLimits, symbolizer: mockSymbolizer, metrics: newQueryFrontendMetrics(nil)}
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_TREE,
		Tree:       &queryv1.TreeReport{Tree: treeBytes, SymbolRefs: pb},
	}

	err := qf.resolveSymbolRefs(context.Background(), []string{"tenant1"}, report, 0)
	require.NoError(t, err)

	names := flameNames(t, report.Tree.Tree)
	require.Contains(t, names, "resolved_b")
	require.Contains(t, names, "libfoo.so!0x100")
	require.Contains(t, names, "libfoo.so!0x200")
	require.Equal(t, float64(2), testutil.ToFloat64(qf.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationTimeout)))

	mockSymbolizer.AssertExpectations(t)
}

// TestResolveBinaries_ParentContextCanceledFailsRequest verifies that when
// the request context itself is canceled (not just a per-binary timebox),
// the resolve error is propagated and the request fails.
func TestResolveBinaries_ParentContextCanceledFailsRequest(t *testing.T) {
	treeBytes, pb := buildSymbolRefFixture(t)

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("SymbolizerResolveTimeout", "tenant1").Return(time.Minute)

	fetchStarted := make(chan struct{})
	var once sync.Once
	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
	mockSymbolizer.On("ResolveConcurrency").Return(4)
	mockSymbolizer.On("Resolve", mock.Anything, "build-a", "libfoo.so", []uint64{0x100, 0x200}).
		Return(func(ctx context.Context, buildID, binaryName string, addrs []uint64) ([][]lidia.SourceInfoFrame, error) {
			once.Do(func() { close(fetchStarted) })
			<-ctx.Done()
			return nil, ctx.Err()
		}).Maybe()
	mockSymbolizer.On("Resolve", mock.Anything, "build-b", "libbar.so", []uint64{0x300}).
		Return(func(ctx context.Context, buildID, binaryName string, addrs []uint64) ([][]lidia.SourceInfoFrame, error) {
			once.Do(func() { close(fetchStarted) })
			<-ctx.Done()
			return nil, ctx.Err()
		}).Maybe()

	qf := &QueryFrontend{limits: mockLimits, symbolizer: mockSymbolizer, metrics: newQueryFrontendMetrics(nil)}
	report := &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_TREE,
		Tree:       &queryv1.TreeReport{Tree: treeBytes, SymbolRefs: pb},
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- qf.resolveSymbolRefs(ctx, []string{"tenant1"}, report, 0)
	}()

	<-fetchStarted
	cancel()
	err := <-errCh

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestResolveBinaries_ZeroResolveTimeoutFallsBackToSafeDefault verifies that
// an all-tenants-zero resolve-timeout limit falls back to a safe positive
// default rather than reproducing the "0 means unlimited/disabled"
// convention used elsewhere: it must not hand Resolve an already-expired
// context.
func TestResolveBinaries_ZeroResolveTimeoutFallsBackToSafeDefault(t *testing.T) {
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("SymbolizerResolveTimeout", "tenant1").Return(time.Duration(0))

	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
	mockSymbolizer.On("ResolveConcurrency").Return(4)
	mockSymbolizer.On("Resolve", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(ctx context.Context, buildID, binaryName string, addrs []uint64) ([][]lidia.SourceInfoFrame, error) {
			require.NoError(t, ctx.Err(), "SymbolizerResolveTimeout=0 must not produce an already-expired context")
			return [][]lidia.SourceInfoFrame{{{FunctionName: "resolved"}}}, nil
		}).Times(2)

	qf := &QueryFrontend{limits: mockLimits, symbolizer: mockSymbolizer, metrics: newQueryFrontendMetrics(nil)}
	binaries := []symbolref.UnresolvedBinary{
		{BuildID: "build-a", BinaryName: "liba.so", Addresses: []uint64{0x1}},
		{BuildID: "build-b", BinaryName: "libb.so", Addresses: []uint64{0x2}},
	}

	_, err := qf.resolveBinaries(context.Background(), []string{"tenant1"}, binaries)
	require.NoError(t, err)
}

// TestResolveBinaries_NonPositiveResolveConcurrencyFallsBackToSafeFloor
// verifies that a Symbolizer reporting a non-positive ResolveConcurrency
// (a defensive case pkg/symbolizer's own normalization should already
// prevent in production) falls back to a safe floor instead of blocking
// errgroup.SetLimit forever.
func TestResolveBinaries_NonPositiveResolveConcurrencyFallsBackToSafeFloor(t *testing.T) {
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("SymbolizerResolveTimeout", "tenant1").Return(time.Second)

	mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
	mockSymbolizer.On("ResolveConcurrency").Return(0)
	mockSymbolizer.On("Resolve", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([][]lidia.SourceInfoFrame{{{FunctionName: "resolved"}}}, nil).Times(2)

	qf := &QueryFrontend{limits: mockLimits, symbolizer: mockSymbolizer, metrics: newQueryFrontendMetrics(nil)}
	binaries := []symbolref.UnresolvedBinary{
		{BuildID: "build-a", BinaryName: "liba.so", Addresses: []uint64{0x1}},
		{BuildID: "build-b", BinaryName: "libb.so", Addresses: []uint64{0x2}},
	}

	done := make(chan error, 1)
	go func() {
		_, err := qf.resolveBinaries(context.Background(), []string{"tenant1"}, binaries)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("resolveBinaries did not return: a non-positive ResolveConcurrency must fall back to a safe floor, not block forever")
	}
}
