package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend/profilediff"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestFunctionProjectionInvalidOptions(t *testing.T) {
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
		MaxNodes:      proto.Int64(1),
	}
	_, err := new(QueryFrontend).SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(req))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestSelectFunctionsLimitsOnlyFinalResult(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	full := &typesv1.FunctionTable{
		Total:          100,
		TotalFunctions: 2,
		Functions: []*typesv1.FunctionStats{{
			Name:  "A",
			Self:  60,
			Total: 60,
		}, {
			Name:  "B",
			Self:  40,
			Total: 40,
		}},
	}
	backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		q := args.Get(1).(*queryv1.InvokeRequest).Query[0]
		require.Equal(t, queryv1.QueryType_QUERY_FUNCTIONS, q.QueryType)
		require.NotNil(t, q.Functions)
	}).Return(&queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
			Functions:  &queryv1.FunctionsReport{Table: full},
		}},
	}, nil)
	q := newSMPQueryFrontend(t, limits, metadata, backend)
	start, end := smpValidTimeRange()
	r := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType,
		LabelSelector: "{}",
		Start:         start,
		End:           end,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
		FormatOptions: &querierv1.ProfileFormatOptions{
			Functions: &querierv1.FunctionsOptions{Limit: 1},
		},
	}
	resp, err := q.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(r))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Functions.Functions, 1)
	require.Equal(t, int64(100), resp.Msg.Functions.Total)
	require.Equal(t, int64(2), resp.Msg.Functions.TotalFunctions)
	require.Len(t, full.Functions, 2)
	require.Same(t, full.Functions[0], resp.Msg.Functions.Functions[0])

	for _, limit := range []int64{0, 1 << 40} {
		r.GetFormatOptions().GetFunctions().Limit = limit
		resp, err = q.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(r))
		require.NoError(t, err)
		require.Same(t, full, resp.Msg.Functions)
	}
}

func TestValidateProjectionRequestRejectsInvalidOptions(t *testing.T) {
	valid := &querierv1.SelectMergeStacktracesRequest{Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS}
	for name, change := range map[string]func(*querierv1.SelectMergeStacktracesRequest){
		"invalid format": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.Format = querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH
		},
		"nil location": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{nil}}
		},
		"empty name": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{}}}
		},
		"max nodes": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.MaxNodes = proto.Int64(1)
		},
		"pgo": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{GoPgo: &typesv1.GoPGO{}}
		},
		"negative limit": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.FormatOptions = &querierv1.ProfileFormatOptions{Functions: &querierv1.FunctionsOptions{Limit: -1}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := valid.CloneVT()
			change(r)
			require.Error(t, validateProjectionRequest(r))
		})
	}
}

func TestValidateProjectionRequestRequiresRequest(t *testing.T) {
	err := validateProjectionRequest(nil)
	require.Error(t, err)
}

func TestProjectionEmptyFunctionsResponse(t *testing.T) {
	req := &querierv1.SelectMergeStacktracesRequest{
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
	}
	err := validateProjectionRequest(req)
	require.NoError(t, err)
	for _, report := range []*queryv1.Report{nil, {}} {
		response := projectionResponse(req, report)
		require.Equal(t, &typesv1.FunctionTable{}, response.Functions)
	}
}

func TestDiffPreservesInvalidLimitForRequestValidation(t *testing.T) {
	req := &querierv1.DiffRequest{
		Left: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: "cpu",
			Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
			FormatOptions: &querierv1.ProfileFormatOptions{
				Functions: &querierv1.FunctionsOptions{Limit: -1},
			},
		},
		Right: &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "cpu"},
	}
	_, err := profilediff.Diff(context.Background(), req, func(_ context.Context, r *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
		require.Equal(t, int64(-1), r.Msg.GetFormatOptions().GetFunctions().Limit)
		err := validateProjectionRequest(r.Msg)
		require.Error(t, err)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	})
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
