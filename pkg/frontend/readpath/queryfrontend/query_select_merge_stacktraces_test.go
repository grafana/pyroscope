package queryfrontend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestSelectMergeStacktraces_RejectsSpanAndTraceSelectors(t *testing.T) {
	qf := new(QueryFrontend)
	resp, err := qf.SelectMergeStacktraces(context.Background(), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		SpanSelector:    []string{"0000000000000001"},
		TraceIdSelector: []string{"00000000000000000000000000000001"},
	}))

	require.Nil(t, resp)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	require.ErrorContains(t, err, "span_selector and trace_id_selector cannot be combined")
}

func TestSelectMergeStacktraces_SpanSelectorTreeFormats(t *testing.T) {
	spanSelector := []string{"0000000000000001"}
	tree := new(phlaremodel.FunctionNameTree)
	tree.InsertStack(1, "foo")
	treeBytes := tree.Bytes(-1, nil)

	tests := []struct {
		name   string
		format querierv1.ProfileFormat
		check  func(*testing.T, *querierv1.SelectMergeStacktracesResponse)
	}{
		{
			name: "unspecified returns flamegraph",
			check: func(t *testing.T, resp *querierv1.SelectMergeStacktracesResponse) {
				require.NotNil(t, resp.Flamegraph)
				require.Empty(t, resp.Tree)
				require.Nil(t, resp.Pprof)
			},
		},
		{
			name:   "flamegraph",
			format: querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH,
			check: func(t *testing.T, resp *querierv1.SelectMergeStacktracesResponse) {
				require.NotNil(t, resp.Flamegraph)
				require.Empty(t, resp.Tree)
				require.Nil(t, resp.Pprof)
			},
		},
		{
			name:   "tree",
			format: querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
			check: func(t *testing.T, resp *querierv1.SelectMergeStacktracesResponse) {
				require.Equal(t, treeBytes, resp.Tree)
				require.Nil(t, resp.Flamegraph)
				require.Nil(t, resp.Pprof)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesDefault", smpTenant).Return(0)
			mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

			mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

			mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockBackend.On("Invoke", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					req := args.Get(1).(*queryv1.InvokeRequest)
					require.Equal(t, spanSelector, req.Query[0].Tree.GetSpanSelector())
				}).
				Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{Tree: treeBytes},
				}}}, nil)

			qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
			ctx := tenant.InjectTenantID(context.Background(), smpTenant)
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: "{}",
				Start:         start,
				End:           end,
				Format:        tt.format,
				SpanSelector:  spanSelector,
			}))

			require.NoError(t, err)
			tt.check(t, resp.Msg)
		})
	}
}

func TestSelectMergeStacktraces_SpanSelectorPprof(t *testing.T) {
	spanSelector := []string{"0000000000000001"}
	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", smpTenant).Return(false)
	mockLimits.On("QueryTreeEnabled", smpTenant).Return(false)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			req := args.Get(1).(*queryv1.InvokeRequest)
			require.Equal(t, queryv1.QueryType_QUERY_PPROF, req.Query[0].QueryType)
			require.Equal(t, spanSelector, req.Query[0].Pprof.GetSpanSelector())
		}).
		Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_PPROF,
			Pprof:      &queryv1.PprofReport{Pprof: createProfile(t)},
		}}}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
		SpanSelector:  spanSelector,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GetPprof().GetProfile())
	require.Len(t, resp.Msg.Pprof.Profile.Sample, 1)
	require.Nil(t, resp.Msg.Flamegraph)
	require.Empty(t, resp.Msg.Tree)
}

func TestSelectMergeStacktrace_Symbolization(t *testing.T) {
	// pprofBytes contains a profile with one sample at location 1, where location 1
	// maps to function "original_func". backendTreeSymbolizer rewrites QUERY_TREE →
	// QUERY_PPROF before calling the upstream backend, so the mock must return this
	// pprof report. The symbolizer mock then renames the function to "symbolized_func",
	// which must be visible in the resulting flamegraph.
	pprofBytes := func() []byte {
		p := &profilev1.Profile{
			StringTable: []string{"", "original_func"},
			Function:    []*profilev1.Function{{Id: 1, Name: 1}},
			Location:    []*profilev1.Location{{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
			Mapping:     []*profilev1.Mapping{{Id: 1}},
			Sample:      []*profilev1.Sample{{LocationId: []uint64{1}, Value: []int64{10}}},
		}
		b, err := pprof.Marshal(p, true)
		require.NoError(t, err)
		return b
	}()

	tests := []struct {
		name             string
		tenantID         string
		hasUnsymbolized  bool
		backendResp      *queryv1.InvokeResponse
		expectSymbolized bool
		setupMocks       func(*mockfrontend.MockLimits, *mockqueryfrontend.MockSymbolizer)
	}{
		{
			name:             "symbolization enabled for tenant with native profiles",
			tenantID:         "tenant1",
			hasUnsymbolized:  true,
			expectSymbolized: true,
			// backendTreeSymbolizer rewrites QUERY_TREE -> QUERY_PPROF before calling
			// the upstream backend, so the mock must return a pprof report.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					Pprof: &queryv1.PprofReport{Pprof: pprofBytes},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant1").Return(true)
				l.On("SymbolRefTreesEnabled", "tenant1").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant1").Return(false)
				s.On("SymbolizePprof", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						p := args.Get(1).(*profilev1.Profile)
						p.StringTable = append(p.StringTable, "symbolized_func")
						p.Function[0].Name = int64(len(p.StringTable) - 1)
					}).
					Return(nil).Once()
			},
		},
		{
			name:             "symbolization disabled for tenant",
			tenantID:         "tenant2",
			hasUnsymbolized:  true,
			expectSymbolized: false,
			// No backendTreeSymbolizer wrapping; backend receives the TREE query directly.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant2").Return(false)
				l.On("SymbolRefTreesEnabled", "tenant2").Return(false).Maybe() // short-circuited when symbolization is off
				l.On("QuerySanitizeOnMerge", "tenant2").Return(false)
			},
		},
		{
			name:             "symbolization enabled but no native profiles",
			tenantID:         "tenant3",
			hasUnsymbolized:  false,
			expectSymbolized: false,
			// No backendTreeSymbolizer wrapping; backend receives the TREE query directly.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant3").Return(true)
				l.On("SymbolRefTreesEnabled", "tenant3").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant3").Return(false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesDefault", tt.tenantID).Return(0)
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(tt.backendResp, nil)

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
							fmt.Sprintf("%v", tt.hasUnsymbolized),
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
				nil,
			)

			ctx := tenant.InjectTenantID(context.Background(), tt.tenantID)
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
			}))

			require.NoError(t, err)
			names := resp.Msg.GetFlamegraph().GetNames()
			if tt.expectSymbolized {
				require.Contains(t, names, "symbolized_func")
				require.NotContains(t, names, "original_func")
			} else {
				require.NotContains(t, names, "symbolized_func")
			}

			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}

func TestSelectMergeStacktraces_DotFormat(t *testing.T) {
	spanSelector := []string{"0000000000000001"}
	p := &profilev1.Profile{
		StringTable: []string{"", "my_func", "samples", "count", "", ""},
		Function:    []*profilev1.Function{{Id: 1, Name: 1}},
		Location:    []*profilev1.Location{{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
		Mapping:     []*profilev1.Mapping{{Id: 1}},
		Sample:      []*profilev1.Sample{{LocationId: []uint64{1}, Value: []int64{10}}},
		PeriodType:  &profilev1.ValueType{Type: 2, Unit: 3},
		SampleType:  []*profilev1.ValueType{{Type: 4, Unit: 5}},
	}
	pprofBytes, err := pprof.Marshal(p, true)
	require.NoError(t, err)

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", "tenant1").Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", "tenant1").Return(false)
	mockLimits.On("QueryTreeEnabled", "tenant1").Return(false)
	mockLimits.On("QuerySanitizeOnMerge", "tenant1").Return(false)

	mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(1).(*queryv1.InvokeRequest)
		require.Equal(t, spanSelector, req.Query[0].Pprof.GetSpanSelector())
	}).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_PPROF,
			Pprof:      &queryv1.PprofReport{Pprof: pprofBytes},
		}},
	}, nil)

	mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).
		Return(&metastorev1.QueryMetadataResponse{
			Blocks: []*metastorev1.BlockMeta{{
				Id:          "block_id",
				Datasets:    []*metastorev1.Dataset{{Labels: []int32{1, 1, 2}}},
				StringTable: []string{"", metadata.LabelNameUnsymbolized, "false"},
			}},
		}, nil)

	qf := NewQueryFrontend(log.NewNopLogger(), mockLimits, mockMetadataClient, nil, mockQueryBackend, nil, nil, nil)

	ctx := tenant.InjectTenantID(context.Background(), "tenant1")
	start, end := smpValidTimeRange()
	resp, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: `{service_name="test-service"}`,
		Start:         start,
		End:           end,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_DOT,
		SpanSelector:  spanSelector,
	}))

	require.NoError(t, err)
	require.NotEmpty(t, resp.Msg.Dot)
	require.Contains(t, resp.Msg.Dot, "digraph")
	require.Nil(t, resp.Msg.Flamegraph)
	require.Empty(t, resp.Msg.Tree)
}
