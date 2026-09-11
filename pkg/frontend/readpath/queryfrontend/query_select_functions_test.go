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

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestSelectFunctions_LimitsAndSelectors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		limit        *int64
		defaultLimit int
		want         int
	}{
		{name: "omitted", defaultLimit: 2000, want: 2000},
		{name: "zero uses tenant default", limit: new(int64(0)), defaultLimit: 1000, want: 1000},
		{name: "unlimited tenant default", want: 2001},
		{name: "explicit", limit: new(int64(1)), want: 1},
		{name: "all", limit: new(int64(-1)), want: 2001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
			limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
			limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
			if tc.limit == nil || *tc.limit == 0 {
				limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(tc.defaultLimit)
			} else {
				limits.On("MaxFlameGraphNodesMax", smpTenant).Return(0)
			}
			metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
			metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
			backend := mockqueryfrontend.NewMockQueryBackend(t)
			rows := make([]*querierv1.FunctionRow, 2001)
			for i := range rows {
				rows[i] = &querierv1.FunctionRow{Name: fmt.Sprintf("function%04d", i), Self: int64(i), Total: int64(i)}
			}
			selector := &typesv1.StackTraceSelector{}
			backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				req := args.Get(1).(*queryv1.InvokeRequest)
				require.Equal(t, []string{smpTenant}, req.Tenant)
				require.Equal(t, queryv1.QueryType_QUERY_FUNCTIONS, req.Query[0].QueryType)
				require.Nil(t, req.Query[0].Tree)
				require.True(t, (&queryv1.FunctionsQuery{
					StackTraceSelector: selector, ProfileIdSelector: []string{"profile-id"}, SpanSelector: []string{"0000000000000001"},
				}).EqualVT(req.Query[0].Functions))
			}).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
				Functions:  &queryv1.FunctionsReport{Functions: &querierv1.FunctionTable{Functions: rows, Total: 2001000}},
			}}}, nil)
			qf := newSMPQueryFrontend(t, limits, metadata, backend)
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
				Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxNodes: tc.limit,
				StackTraceSelector: selector, ProfileIdSelector: []string{"profile-id"}, SpanSelector: []string{"0000000000000001"},
			}))
			require.NoError(t, err)
			require.Len(t, resp.Msg.Functions.Functions, tc.want)
			require.Equal(t, "function2000", resp.Msg.Functions.Functions[0].Name)
			require.Equal(t, int64(2000), resp.Msg.Functions.Functions[0].Self)
			require.Equal(t, int64(2000), resp.Msg.Functions.Functions[0].Total)
			require.Equal(t, int64(2001000), resp.Msg.Functions.Total)
			require.Nil(t, resp.Msg.Flamegraph)
			require.Empty(t, resp.Msg.Tree)
		})
	}
}

func TestSelectFunctions_InvalidLimit(t *testing.T) {
	for _, limit := range []int64{11, -1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
			limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
			limits.On("MaxFlameGraphNodesMax", smpTenant).Return(10)
			qf := newSMPQueryFrontend(t, limits, new(mockmetastorev1.MockMetadataQueryServiceClient), mockqueryfrontend.NewMockQueryBackend(t))
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
				Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxNodes: &limit,
			}))
			require.Nil(t, resp)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			require.ErrorContains(t, err, "max flamegraph nodes limit")
		})
	}
}

func TestSelectFunctions_Empty(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(2000)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{}, nil)
	qf := newSMPQueryFrontend(t, limits, metadata, mockqueryfrontend.NewMockQueryBackend(t))
	start, end := smpValidTimeRange()
	resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end, Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Functions)
	require.Empty(t, resp.Msg.Functions.Functions)
	require.Zero(t, resp.Msg.Functions.Total)
}

func TestSelectFunctions_NativeSymbolizationOmitted(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxFlameGraphNodesMax", smpTenant).Return(1000)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
	limits.On("SymbolizerEnabled", smpTenant).Return(true).Maybe()
	limits.On("SymbolRefTreesEnabled", smpTenant).Return(true).Maybe()
	// Even with a symbolizer configured, function queries must use backend
	// function reports without invoking native symbolization.
	sym := mockqueryfrontend.NewMockSymbolizer(t)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(1).(*queryv1.InvokeRequest)
		require.Equal(t, queryv1.QueryType_QUERY_FUNCTIONS, req.Query[0].QueryType)
		require.Nil(t, req.Query[0].Tree)
	}).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
		Functions: &queryv1.FunctionsReport{Functions: &querierv1.FunctionTable{
			Total: 10, Functions: []*querierv1.FunctionRow{{Name: "known_func", Self: 7, Total: 10}},
		}},
	}}}, nil)
	qf := NewQueryFrontend(log.NewNopLogger(), limits, frontend.Config{}, metadata, nil, backend, sym, nil, nil)
	start, end := smpValidTimeRange()
	maxNodes := int64(1)
	resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxNodes: &maxNodes,
	}))
	require.NoError(t, err)
	require.Equal(t, int64(10), resp.Msg.Functions.Total)
	require.Equal(t, []*querierv1.FunctionRow{{Name: "known_func", Self: 7, Total: 10}}, resp.Msg.Functions.Functions)
}
