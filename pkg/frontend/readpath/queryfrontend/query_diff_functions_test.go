package queryfrontend

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestDiffFunctions_CompleteQueriesAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit *int64
		want  int
	}{
		{"default", nil, 1}, {"explicit", new(int64(2)), 2}, {"unlimited", new(int64(-1)), 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
			limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
			limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
			if tc.limit == nil {
				limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(1)
			} else {
				limits.On("MaxFlameGraphNodesMax", smpTenant).Return(0)
			}
			metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
			metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
			backend := mockqueryfrontend.NewMockQueryBackend(t)
			leftQuery := &queryv1.FunctionsQuery{SpanSelector: []string{"0000000000000001"}, ProfileIdSelector: []string{"550e8400-e29b-41d4-a716-446655440000"}}
			rightQuery := &queryv1.FunctionsQuery{TraceIdSelector: []string{"0123456789abcdef0123456789abcdef"}, StackTraceSelector: &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{Name: "root"}}}}
			backend.On("Invoke", mock.Anything, mock.Anything).Return(func(_ context.Context, req *queryv1.InvokeRequest) *queryv1.InvokeResponse {
				require.Equal(t, []string{smpTenant}, req.Tenant)
				require.Equal(t, queryv1.QueryType_QUERY_FUNCTIONS, req.Query[0].QueryType)
				require.Nil(t, req.Query[0].Tree)
				wantQuery := rightQuery
				table := &querierv1.FunctionTable{Total: 1000, Functions: []*querierv1.FunctionRow{
					{Name: "shared", Self: 1, Total: 900}, {Name: "right", Self: 999, Total: 999},
				}}
				if strings.Contains(req.LabelSelector, `side="left"`) {
					wantQuery = leftQuery
					table = &querierv1.FunctionTable{Total: 100, Functions: []*querierv1.FunctionRow{
						{Name: "shared", Self: 1, Total: 100}, {Name: "left", Self: 99, Total: 99},
					}}
				}
				require.True(t, wantQuery.EqualVT(req.Query[0].Functions))
				return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
					Functions:  &queryv1.FunctionsReport{Query: wantQuery.CloneVT(), Functions: table},
				}}}
			}, nil).Twice()
			q := newSMPQueryFrontend(t, limits, metadata, backend)
			start, end := smpValidTimeRange()
			req := &querierv1.DiffRequest{
				Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxNodes: tc.limit,
				Left: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: smpProfileType, LabelSelector: `{side="left"}`, Start: start, End: end,
					MaxNodes: new(int64(1)), Format: querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
					SpanSelector: leftQuery.SpanSelector, ProfileIdSelector: leftQuery.ProfileIdSelector,
				},
				Right: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: smpProfileType, LabelSelector: `{side="right"}`, Start: start, End: end,
					MaxNodes: new(int64(1)), TraceIdSelector: rightQuery.TraceIdSelector, StackTraceSelector: rightQuery.StackTraceSelector,
				},
			}
			original := req.CloneVT()
			resp, err := q.Diff(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(req))
			require.NoError(t, err)
			require.True(t, original.EqualVT(req))
			require.Nil(t, resp.Msg.Flamegraph)
			require.Len(t, resp.Msg.Functions.Functions, tc.want)
			require.Equal(t, int64(100), resp.Msg.Functions.LeftTotal)
			require.Equal(t, int64(1000), resp.Msg.Functions.RightTotal)
			require.Equal(t, &querierv1.FunctionDiffRow{Name: "shared", LeftSelf: 1, LeftTotal: 100, RightSelf: 1, RightTotal: 900}, resp.Msg.Functions.Functions[0])
		})
	}
}

func TestDiffFunctions_EmptyAndAliasedSelections(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(10)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{}, nil)
	q := newSMPQueryFrontend(t, limits, metadata, mockqueryfrontend.NewMockQueryBackend(t))
	start, end := smpValidTimeRange()
	selection := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end}
	resp, err := q.Diff(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.DiffRequest{
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, Left: selection, Right: selection,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Functions)
	require.Empty(t, resp.Msg.Functions.Functions)
	require.Zero(t, resp.Msg.Functions.LeftTotal)
	require.Zero(t, resp.Msg.Functions.RightTotal)
}

func TestDiffFunctions_Validation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*querierv1.DiffRequest)
	}{
		{"missing left", func(r *querierv1.DiffRequest) { r.Left = nil }},
		{"missing right", func(r *querierv1.DiffRequest) { r.Right = nil }},
		{"invalid type", func(r *querierv1.DiffRequest) { r.Left.ProfileTypeID = "invalid" }},
		{"different types", func(r *querierv1.DiffRequest) { r.Right.ProfileTypeID = "memory:alloc_space:bytes:space:bytes" }},
		{"async", func(r *querierv1.DiffRequest) { r.Left.Async = new(querierv1.AsyncQueryRequest) }},
		{"limit exceeded", func(r *querierv1.DiffRequest) { r.MaxNodes = new(int64(11)) }},
		{"unlimited forbidden", func(r *querierv1.DiffRequest) { r.MaxNodes = new(int64(-1)) }},
		{"unsupported format", func(r *querierv1.DiffRequest) { r.Format = querierv1.ProfileFormat_PROFILE_FORMAT_TREE }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := mockfrontend.NewMockLimits(t)
			limits.On("MaxFlameGraphNodesMax", smpTenant).Return(10).Maybe()
			q := newSMPQueryFrontend(t, limits, new(mockmetastorev1.MockMetadataQueryServiceClient), mockqueryfrontend.NewMockQueryBackend(t))
			req := &querierv1.DiffRequest{
				Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
				Left:   &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: smpProfileType},
				Right:  &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: smpProfileType},
			}
			tc.modify(req)
			resp, err := q.Diff(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(req))
			require.Nil(t, resp)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestDiffFunctions_BackendError(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(10)
	limits.On("QuerySanitizeOnMerge", smpTenant).Return(false).Maybe()
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.Anything).Return(nil, context.DeadlineExceeded)
	q := newSMPQueryFrontend(t, limits, metadata, backend)
	start, end := smpValidTimeRange()
	selection := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end}
	resp, err := q.Diff(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.DiffRequest{
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, Left: selection, Right: selection,
	}))
	require.Nil(t, resp)
	require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
}

func TestDiffFunctions_LookbackAndFederation(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	for _, id := range []string{"tenant-a", "tenant-b"} {
		limits.On("MaxQueryLookback", id).Return(time.Hour)
		limits.On("MaxQueryLength", id).Return(time.Duration(0))
		limits.On("MaxFlameGraphNodesDefault", id).Return(1)
	}
	limits.On("QuerySanitizeOnMerge", "tenant-a").Return(false)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil).Once()
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	now := time.Now()
	backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(1).(*queryv1.InvokeRequest)
		require.Equal(t, []string{"tenant-a", "tenant-b"}, req.Tenant)
		require.GreaterOrEqual(t, req.StartTime, now.Add(-time.Hour).UnixMilli())
	}).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
		Functions: &queryv1.FunctionsReport{Functions: &querierv1.FunctionTable{
			Total: 10, Functions: []*querierv1.FunctionRow{{Name: "new", Self: 10, Total: 10}},
		}},
	}}}, nil).Once()
	q := newSMPQueryFrontend(t, limits, metadata, backend)
	left := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}",
		Start: now.Add(-3 * time.Hour).UnixMilli(), End: now.Add(-2 * time.Hour).UnixMilli(),
	}
	right := left.CloneVT()
	right.End = now.UnixMilli()
	req := &querierv1.DiffRequest{Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, Left: left, Right: right}
	original := req.CloneVT()
	resp, err := q.Diff(tenant.InjectTenantID(context.Background(), "tenant-a|tenant-b"), connect.NewRequest(req))
	require.NoError(t, err)
	require.True(t, original.EqualVT(req))
	require.Zero(t, resp.Msg.Functions.LeftTotal)
	require.Equal(t, int64(10), resp.Msg.Functions.RightTotal)
	require.Equal(t, []*querierv1.FunctionDiffRow{{Name: "new", RightSelf: 10, RightTotal: 10}}, resp.Msg.Functions.Functions)
	metadata.AssertExpectations(t)
}
