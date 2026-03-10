package queryfrontend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockqueryfrontend"
)

const (
	smpTenant = "test"
)

func newSMPQueryFrontend(
	t *testing.T,
	limits *mockfrontend.MockLimits,
	metaClient *mockmetastorev1.MockMetadataQueryServiceClient,
	backend *mockqueryfrontend.MockQueryBackend,
) *QueryFrontend {
	t.Helper()
	return NewQueryFrontend(
		log.NewNopLogger(),
		limits,
		metaClient,
		nil, // tenantServiceClient
		backend,
		nil, // symbolizer
		nil, // diagnosticsStore
	)
}

func smpOneBlock() *metastorev1.QueryMetadataResponse {
	return &metastorev1.QueryMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: "block-a"}},
	}
}

// createProfile returns a serialised pprof profile with a single sample.
func createProfile(t *testing.T) []byte {
	t.Helper()
	p := &profilev1.Profile{
		Sample: []*profilev1.Sample{{Value: []int64{10}}},
	}
	b, err := pprof.Marshal(p, true)
	require.NoError(t, err)
	return b
}

func TestSelectMergeProfile_EmptyRangeReturnsEmptyProfile(t *testing.T) {
	// MaxQueryLookback=24h and Start/End at 1ms/1s after epoch, which is well
	// outside the lookback window.  The frontend should return an empty profile
	// without ever contacting the query backend.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Hour * 24)

	qf := newSMPQueryFrontend(t, mockLimits,
		new(mockmetastorev1.MockMetadataQueryServiceClient),
		new(mockqueryfrontend.MockQueryBackend))

	ctx := user.InjectOrgID(context.Background(), smpTenant)
	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         1,
		End:           1000,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.Empty(t, resp.Msg.Sample)
}

func TestSelectMergeProfile_InvalidProfileTypeReturnsError(t *testing.T) {
	// SanitizeTimeRange is called first (needs MaxQueryLookback + MaxQueryLength),
	// then ParseProfileTypeSelector fails and the function returns an error.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))

	qf := newSMPQueryFrontend(t, mockLimits,
		new(mockmetastorev1.MockMetadataQueryServiceClient),
		new(mockqueryfrontend.MockQueryBackend))

	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	_, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: "invalid", // not the required name:sampleType:sampleUnit:periodType:periodUnit form
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.Error(t, err)
	require.ErrorContains(t, err, "profile-type selection must be of the form")
}

func TestSelectMergeProfile_PprofPath_NoBlocks(t *testing.T) {
	// When QueryMetadata returns no blocks, querySingle returns nil and
	// SelectMergeProfile must return an empty profile without error.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).
		Return(&metastorev1.QueryMetadataResponse{}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, new(mockqueryfrontend.MockQueryBackend))

	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.Empty(t, resp.Msg.Sample)
}

func TestSelectMergeProfile_PprofPath_ReturnsPprof(t *testing.T) {
	// Happy path for the pprof query-backend path: the backend returns a
	// serialised pprof report which is unmarshalled and forwarded.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(false)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_PPROF,
			Pprof:      &queryv1.PprofReport{Pprof: createProfile(t)},
		}},
	}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	// createProfile builds a profile with one sample.
	require.Len(t, resp.Msg.Sample, 1)
}

func TestSelectMergeProfile_PprofPath_SendsQueryPprof(t *testing.T) {
	// Ensure the pprof path sends a QUERY_PPROF request to the backend (not QUERY_TREE).
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(false)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var observedQueryType queryv1.QueryType
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			req := args.Get(1).(*queryv1.InvokeRequest)
			if len(req.Query) > 0 {
				observedQueryType = req.Query[0].QueryType
			}
		}).
		Return(&queryv1.InvokeResponse{
			Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_PPROF,
				Pprof:      &queryv1.PprofReport{Pprof: createProfile(t)},
			}},
		}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	_, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	assert.Equal(t, queryv1.QueryType_QUERY_PPROF, observedQueryType)
}

func TestSelectMergeProfile_TreePath_NoBlocks(t *testing.T) {
	// QueryTreeEnabled=true but no blocks found: must return empty profile without error.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(true)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).
		Return(&metastorev1.QueryMetadataResponse{}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, new(mockqueryfrontend.MockQueryBackend))
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.Empty(t, resp.Msg.Sample)
}

func TestSelectMergeProfile_TreePath_ReconstructsProfile(t *testing.T) {
	// Happy path for the tree query-backend path: the backend returns a
	// LocationRefName tree plus TreeSymbols; SelectMergeProfile must reconstruct
	// a valid pprof profile from them.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(true)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	// Build a LocationRefNameTree with a single stack: 10 samples at location ref 1.
	lrTree := new(phlaremodel.LocationRefNameTree)
	lrTree.InsertStack(10, phlaremodel.LocationRefName(1))
	treeBytes := lrTree.Bytes(-1, func(n phlaremodel.LocationRefName) phlaremodel.LocationRefName { return n })

	// Symbols table: index 0 is the sentinel, index 1 is the real entry.
	symbols := &queryv1.TreeSymbols{
		Strings:  []string{"", "funcname"},
		Mappings: []*profilev1.Mapping{{Id: 0}},
		Locations: []*profilev1.Location{
			{Id: 0},
			{Id: 1},
		},
		Functions: []*profilev1.Function{{Id: 0}},
	}

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_TREE,
			Tree: &queryv1.TreeReport{
				Tree:    treeBytes,
				Symbols: symbols,
			},
		}},
	}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	now := time.Now().UnixMilli()

	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         now,
		End:           now + time.Minute.Milliseconds(),
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	// One sample at location 1 with value 10.
	require.Len(t, resp.Msg.Sample, 1)
	require.Equal(t, int64(10), resp.Msg.Sample[0].Value[0])
	require.Equal(t, []uint64{1}, resp.Msg.Sample[0].LocationId)

	// Sentinel is stripped: p.Location = Symbols.Locations[1:] = [{Id:1}]
	require.Len(t, resp.Msg.Location, 1)
	require.Equal(t, uint64(1), resp.Msg.Location[0].Id)

	// The profile type components are appended to the string table.
	require.Contains(t, resp.Msg.StringTable, "inuse_space") // SampleType
	require.Contains(t, resp.Msg.StringTable, "bytes")       // SampleUnit
	require.Contains(t, resp.Msg.StringTable, "space")       // PeriodType

	// SampleType and PeriodType point into the string table.
	require.Len(t, resp.Msg.SampleType, 1)
	require.NotNil(t, resp.Msg.PeriodType)
}

func TestSelectMergeProfile_TreePath_SendsQueryTreeWithFullSymbols(t *testing.T) {
	// Ensure the tree path sends QUERY_TREE with FullSymbols=true to the backend.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(true)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	lrTree := new(phlaremodel.LocationRefNameTree)
	lrTree.InsertStack(5, phlaremodel.LocationRefName(1))
	treeBytes := lrTree.Bytes(-1, func(n phlaremodel.LocationRefName) phlaremodel.LocationRefName { return n })

	symbols := &queryv1.TreeSymbols{
		Strings:   []string{""},
		Mappings:  []*profilev1.Mapping{{Id: 0}},
		Locations: []*profilev1.Location{{Id: 0}, {Id: 1}},
		Functions: []*profilev1.Function{{Id: 0}},
	}

	var capturedQuery *queryv1.Query
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			req := args.Get(1).(*queryv1.InvokeRequest)
			if len(req.Query) > 0 {
				capturedQuery = req.Query[0]
			}
		}).
		Return(&queryv1.InvokeResponse{
			Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_TREE,
				Tree:       &queryv1.TreeReport{Tree: treeBytes, Symbols: symbols},
			}},
		}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	_, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
	}))

	require.NoError(t, err)
	require.NotNil(t, capturedQuery)
	assert.Equal(t, queryv1.QueryType_QUERY_TREE, capturedQuery.QueryType)
	assert.True(t, capturedQuery.Tree.GetFullSymbols(), "tree path must request full symbols")
}

func TestSelectMergeProfile_TreePath_OtherLocationRef(t *testing.T) {
	// When a tree contains OtherLocationRef nodes (produced by node truncation),
	// selectMergeProfileTree must synthesise a single "other" location/function
	// and reference it from every affected sample.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(true)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	// Two stacks that both contain OtherLocationRef:
	//   stack A: [OtherLocationRef]               self=3  (pure "other")
	//   stack B: [OtherLocationRef, LocationRef(1)] self=7  (mixed real + other)
	lrTree := new(phlaremodel.LocationRefNameTree)
	lrTree.InsertStack(3, phlaremodel.OtherLocationRef)
	lrTree.InsertStack(7, phlaremodel.LocationRefName(1), phlaremodel.OtherLocationRef)
	treeBytes := lrTree.Bytes(-1, func(n phlaremodel.LocationRefName) phlaremodel.LocationRefName { return n })

	// Symbols contain one real location at index 1; index 0 is the sentinel.
	symbols := &queryv1.TreeSymbols{
		Strings:  []string{"", "funcname"},
		Mappings: []*profilev1.Mapping{{Id: 0}},
		Locations: []*profilev1.Location{
			{Id: 0},
			{Id: 1},
		},
		Functions: []*profilev1.Function{{Id: 0}},
	}

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_TREE,
			Tree:       &queryv1.TreeReport{Tree: treeBytes, Symbols: symbols},
		}},
	}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	now := time.Now().UnixMilli()

	resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         now,
		End:           now + time.Minute.Milliseconds(),
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	// Exactly two samples, one per stack.
	require.Len(t, resp.Msg.Sample, 2)

	// The synthetic "other" string is present exactly once.
	otherCount := 0
	for _, s := range resp.Msg.StringTable {
		if s == "other" {
			otherCount++
		}
	}
	require.Equal(t, 1, otherCount, "synthetic 'other' string must be added exactly once")

	// Exactly one synthetic "other" Location was appended (Symbols.Locations[1:] has
	// one entry, so after appending "other" there are two).
	require.Len(t, resp.Msg.Location, 2)
	otherLoc := resp.Msg.Location[1]

	// Exactly one synthetic "other" Function was appended (Symbols.Functions[1:] is
	// empty, so there is exactly one entry afterwards).
	require.Len(t, resp.Msg.Function, 1)
	require.Len(t, otherLoc.Line, 1)
	require.Equal(t, resp.Msg.Function[0].Id, otherLoc.Line[0].FunctionId)

	// Both samples reference the same synthetic "other" location ID.
	for i, s := range resp.Msg.Sample {
		found := false
		for _, locID := range s.LocationId {
			if locID == otherLoc.Id {
				found = true
				break
			}
		}
		require.True(t, found, "sample %d should reference the synthetic other location", i)
	}
}

func TestSelectMergeProfile_PGOBypassesTreePath(t *testing.T) {
	// When a GoPGO stack-trace selector is present, the pprof path must be used
	// even when QueryTreeEnabled returns true.
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	// QueryTreeEnabled is NOT called when a PGO selector is present; the code
	// short-circuits before checking it.
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	var observedQueryType queryv1.QueryType
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			req := args.Get(1).(*queryv1.InvokeRequest)
			if len(req.Query) > 0 {
				observedQueryType = req.Query[0].QueryType
			}
		}).
		Return(&queryv1.InvokeResponse{
			Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_PPROF,
				Pprof:      &queryv1.PprofReport{Pprof: createProfile(t)},
			}},
		}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := user.InjectOrgID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()

	_, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
		StackTraceSelector: &typesv1.StackTraceSelector{
			GoPgo: &typesv1.GoPGO{},
		},
	}))

	require.NoError(t, err)
	assert.Equal(t, queryv1.QueryType_QUERY_PPROF, observedQueryType,
		"GoPGO selector must bypass the tree path and use QUERY_PPROF")
}

func TestSelectMergeProfiles_Symbolization(t *testing.T) {
	// Backend response for the pprof path.
	pprofBackendResp := &queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_PPROF,
			Pprof:      &queryv1.PprofReport{Pprof: createProfile(t)},
		}},
	}

	// Backend response for the tree path.
	lrTree := new(phlaremodel.LocationRefNameTree)
	lrTree.InsertStack(10, phlaremodel.LocationRefName(1))
	treeBytes := lrTree.Bytes(-1, func(n phlaremodel.LocationRefName) phlaremodel.LocationRefName { return n })
	treeBackendResp := &queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_TREE,
			Tree: &queryv1.TreeReport{
				Tree: treeBytes,
				Symbols: &queryv1.TreeSymbols{
					Strings:   []string{"", "funcname"},
					Mappings:  []*profilev1.Mapping{{Id: 0}, {Id: 1, HasFunctions: false}},
					Locations: []*profilev1.Location{{Id: 0}, {Id: 1, MappingId: 1}},
					Functions: []*profilev1.Function{{Id: 0}},
				},
			},
		}},
	}

	tests := []struct {
		name            string
		tenantID        string
		useTree         bool
		hasUnsymbolized bool
		setupMocks      func(*mockfrontend.MockLimits, *mockqueryfrontend.MockSymbolizer)
		verifyResult    func(*testing.T, *mockqueryfrontend.MockSymbolizer, *connect.Response[profilev1.Profile])
	}{
		{
			name:            "pprof: enabled with unsymbolized profiles",
			tenantID:        "tenant1",
			hasUnsymbolized: true,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant1").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant1").Return(false)
				s.On("SymbolizePprof", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						p := args.Get(1).(*profilev1.Profile)
						p.StringTable = append(p.StringTable, "symbolized")
					}).
					Return(nil).Once()
			},
			verifyResult: func(t *testing.T, _ *mockqueryfrontend.MockSymbolizer, resp *connect.Response[profilev1.Profile]) {
				require.Contains(t, resp.Msg.StringTable, "symbolized")
			},
		},
		{
			name:            "pprof: disabled",
			tenantID:        "tenant2",
			hasUnsymbolized: true,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant2").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant2").Return(false)
			},
			verifyResult: func(t *testing.T, _ *mockqueryfrontend.MockSymbolizer, resp *connect.Response[profilev1.Profile]) {
				require.NotContains(t, resp.Msg.StringTable, "symbolized")
			},
		},
		{
			name:            "pprof: enabled but no unsymbolized profiles",
			tenantID:        "tenant3",
			hasUnsymbolized: false,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant3").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant3").Return(false)
			},
			verifyResult: func(t *testing.T, _ *mockqueryfrontend.MockSymbolizer, resp *connect.Response[profilev1.Profile]) {
				require.NotContains(t, resp.Msg.StringTable, "symbolized")
			},
		},
		{
			name:            "tree: enabled with unsymbolized profiles",
			tenantID:        "tenant4",
			useTree:         true,
			hasUnsymbolized: true,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant4").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant4").Return(false)
				s.On("SymbolizePprof", mock.Anything, mock.Anything).Return(nil).Once()
			},
			verifyResult: func(t *testing.T, s *mockqueryfrontend.MockSymbolizer, _ *connect.Response[profilev1.Profile]) {
				s.AssertCalled(t, "SymbolizePprof", mock.Anything, mock.Anything)
			},
		},
		{
			name:            "tree: disabled",
			tenantID:        "tenant5",
			useTree:         true,
			hasUnsymbolized: true,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant5").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant5").Return(false)
			},
			verifyResult: func(t *testing.T, s *mockqueryfrontend.MockSymbolizer, _ *connect.Response[profilev1.Profile]) {
				s.AssertNotCalled(t, "SymbolizePprof", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", tt.tenantID).Return(false)
			mockLimits.On("QueryTreeEnabled", tt.tenantID).Return(tt.useTree)
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			backendResp := pprofBackendResp
			if tt.useTree {
				backendResp = treeBackendResp
			}
			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(backendResp, nil)

			unsymbolizedValue := fmt.Sprintf("%v", tt.hasUnsymbolized)
			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).
				Return(&metastorev1.QueryMetadataResponse{
					Blocks: []*metastorev1.BlockMeta{{
						Id: "block_id",
						Datasets: []*metastorev1.Dataset{{
							Labels: []int32{1, 1, 2},
						}},
						StringTable: []string{
							"", // First string is always empty by convention
							metadata.LabelNameUnsymbolized,
							unsymbolizedValue,
						},
					}},
				}, nil).
				Once()

			qf := NewQueryFrontend(
				log.NewNopLogger(),
				mockLimits,
				mockMetadataClient,
				nil,
				mockQueryBackend,
				mockSymbolizer,
				nil,
			)

			ctx := tenant.InjectTenantID(context.Background(), tt.tenantID)
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
			}))

			require.NoError(t, err)
			tt.verifyResult(t, mockSymbolizer, resp)

			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}
