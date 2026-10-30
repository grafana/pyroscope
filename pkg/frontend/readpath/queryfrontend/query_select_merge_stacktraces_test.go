package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
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
	mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	qf := NewQueryFrontend(log.NewNopLogger(), mockLimits, frontend.Config{}, 30*time.Second, mockMetadataClient, nil, mockQueryBackend, nil, nil)

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
