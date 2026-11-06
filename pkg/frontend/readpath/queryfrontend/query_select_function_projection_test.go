package queryfrontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
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

func TestSelectFunctionTreeForwarding(t *testing.T) {
	for _, direction := range []typesv1.FunctionTreeDirection{typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS} {
		for _, depth := range []*int32{nil, proto.Int32(0), proto.Int32(4)} {
			t.Run(direction.String(), func(t *testing.T) {
				limits := mockfrontend.NewMockLimits(t)
				limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
				limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
				limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
				metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
				metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
				backend := mockqueryfrontend.NewMockQueryBackend(t)
				start, end := smpValidTimeRange()
				req := &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: smpProfileType,
					LabelSelector: "{}",
					Start:         start,
					End:           end,
					Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE,
					FormatOptions: &querierv1.ProfileFormatOptions{
						FunctionTree: &typesv1.FunctionTreeOptions{
							Direction: direction,
							Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
							MaxDepth:  depth,
						},
					},
					StackTraceSelector: &typesv1.StackTraceSelector{
						CallSite: []*typesv1.Location{{Name: "F"}, {Name: "caller"}},
					},
					SpanSelector:      []string{"0000000000000001"},
					ProfileIdSelector: []string{"7c9e6679-7425-40de-944b-e07fc1f90ae7"},
				}
				if direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS {
					req.SpanSelector = nil
					req.TraceIdSelector = []string{"00000000000000000000000000000001"}
				}
				original := req.CloneVT()
				want := &typesv1.FunctionTree{}
				branch := &typesv1.CallTree{
					Root: &typesv1.CallTreeNode{
						Name:        "caller",
						Total:       100,
						HasChildren: true,
					},
				}
				if direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS {
					want.Callers = branch
				} else {
					want.Callees = branch
				}
				backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					q := args.Get(1).(*queryv1.InvokeRequest).Query[0].FunctionTree
					require.True(t, req.GetFormatOptions().GetFunctionTree().EqualVT(q.Options))
					require.Equal(t, req.SpanSelector, q.SpanSelector)
					require.Equal(t, req.ProfileIdSelector, q.ProfileIdSelector)
					require.Equal(t, req.TraceIdSelector, q.TraceIdSelector)
					require.True(t, req.StackTraceSelector.EqualVT(q.StackTraceSelector))
				}).Return(&queryv1.InvokeResponse{
					Reports: []*queryv1.Report{{
						ReportType:   queryv1.ReportType_REPORT_FUNCTION_TREE,
						FunctionTree: &queryv1.FunctionTreeReport{Tree: want},
					}},
				}, nil)
				q := newSMPQueryFrontend(t, limits, metadata, backend)
				direct, err := q.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(req))
				require.NoError(t, err)
				require.Same(t, want, direct.Msg.FunctionTree)
				handler := connect.NewUnaryHandler(querierv1connect.QuerierServiceSelectMergeStacktracesProcedure, q.SelectMergeStacktraces)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handler.ServeHTTP(w, r.WithContext(tenant.InjectTenantID(r.Context(), smpTenant)))
				}))
				t.Cleanup(server.Close)
				client := querierv1connect.NewQuerierServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
				resp, err := client.SelectMergeStacktraces(context.Background(), connect.NewRequest(req))
				require.NoError(t, err)
				require.True(t, want.EqualVT(resp.Msg.FunctionTree))
				require.True(t, original.EqualVT(req))
				require.Nil(t, resp.Msg.Flamegraph)
			})
		}
	}
}

func TestValidateProjectionRequestRejectsInvalidOptions(t *testing.T) {
	valid := &querierv1.SelectMergeStacktracesRequest{
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE,
		FormatOptions: &querierv1.ProfileFormatOptions{
			FunctionTree: &typesv1.FunctionTreeOptions{
				Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
				Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
			},
		},
	}
	for name, change := range map[string]func(*querierv1.SelectMergeStacktracesRequest){
		"invalid format": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.Format = querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH
		},
		"nil location": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{nil}}
		},
		"nil tree options": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.FormatOptions.FunctionTree = nil
		},
		"negative depth": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().MaxDepth = proto.Int32(-1)
		},
		"too deep": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().MaxDepth = proto.Int32(129)
		},
		"missing options": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.FormatOptions = nil
		},
		"missing selection": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().Selection = 0
		},
		"invalid direction": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().Direction = 99
		},
		"root callers": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().Direction = typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS
		},
		"missing function": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().Selection = typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN
		},
		"ambiguous both chain": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.GetFormatOptions().GetFunctionTree().Selection = typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN
			r.GetFormatOptions().GetFunctionTree().Direction = typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH
			r.StackTraceSelector = &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "F"}, {Name: "A"}},
			}
		},
		"max nodes": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.MaxNodes = proto.Int64(1)
		},
		"empty name": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{}},
			}
		},
		"pgo": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.StackTraceSelector = &typesv1.StackTraceSelector{
				GoPgo: &typesv1.GoPGO{},
			}
		},
		"wrong option branch": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.FormatOptions = &querierv1.ProfileFormatOptions{
				Functions: &querierv1.FunctionsOptions{},
			}
		},
		"negative limit": func(r *querierv1.SelectMergeStacktracesRequest) {
			r.Format = querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS
			r.FormatOptions = &querierv1.ProfileFormatOptions{
				Functions: &querierv1.FunctionsOptions{Limit: -1},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := valid.CloneVT()
			change(r)
			err := validateProjectionRequest(r)
			require.Error(t, err)
		})
	}
}

func TestValidateProjectionRequestRequiresRequest(t *testing.T) {
	err := validateProjectionRequest(nil)
	require.Error(t, err)
}

func TestValidateProjectionRequestIgnoresUnusedFormatOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
	}{
		{
			name: "functions ignores invalid tree options",
			json: `{
				"format": "PROFILE_FORMAT_FUNCTIONS",
				"formatOptions": {
					"functionTree": {"maxDepth": -1},
					"functions": {"limit": 1}
				}
			}`,
		},
		{
			name: "tree ignores invalid functions options",
			json: `{
				"format": "PROFILE_FORMAT_FUNCTION_TREE",
				"formatOptions": {
					"functionTree": {
						"direction": "FUNCTION_TREE_DIRECTION_CALLEES",
						"selection": "FUNCTION_TREE_SELECTION_ROOT_PATH"
					},
					"functions": {"limit": -1}
				}
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req querierv1.SelectMergeStacktracesRequest
			require.NoError(t, protojson.Unmarshal([]byte(tc.json), &req))
			original := req.CloneVT()
			err := validateProjectionRequest(&req)
			require.NoError(t, err)
			require.True(t, original.EqualVT(&req))
		})
	}
}

func TestProjectionSelectionAndEmptyResponse(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direction typesv1.FunctionTreeDirection
		selection typesv1.FunctionTreeSelection
		path      []string
	}{
		{"root", typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, nil},
		{"root path", typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, []string{"main", "F"}},
		{"callees", typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, []string{"F", "leaf"}},
		{"callers", typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, []string{"caller", "F"}},
		{"both", typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, []string{"F"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, depth := range []*int32{nil, proto.Int32(0), proto.Int32(128)} {
				options := &typesv1.FunctionTreeOptions{
					Direction: tc.direction,
					Selection: tc.selection,
					MaxDepth:  depth,
				}
				selector := &typesv1.StackTraceSelector{}
				for _, name := range tc.path {
					selector.CallSite = append(selector.CallSite, &typesv1.Location{Name: name})
				}
				req := &querierv1.SelectMergeStacktracesRequest{
					Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE,
					FormatOptions: &querierv1.ProfileFormatOptions{
						FunctionTree: options,
					},
					StackTraceSelector: selector,
				}
				original := req.CloneVT()
				err := validateProjectionRequest(req)
				require.NoError(t, err)
				require.True(t, original.EqualVT(req))
				for _, report := range []*queryv1.Report{nil, {}} {
					tree := projectionResponse(req, report).FunctionTree
					require.Equal(t, tc.direction != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, tree.Callers != nil)
					require.Equal(t, tc.direction != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, tree.Callees != nil)
					require.Nil(t, tree.GetCallers().GetRoot())
					require.Nil(t, tree.GetCallees().GetRoot())
				}
			}
		})
	}
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
		require.Nil(t, response.FunctionTree)
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
