package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func TestSelectMergeSpanProfile_Formats(t *testing.T) {
	spanSelector := []string{"span-abc123"}
	tree := new(phlaremodel.FunctionNameTree)
	tree.InsertStack(1, "foo")
	treeBytes := tree.Bytes(-1, nil)

	tests := []struct {
		name   string
		format querierv1.ProfileFormat
		check  func(*testing.T, *querierv1.SelectMergeSpanProfileResponse)
	}{
		{
			name: "unspecified returns flamegraph",
			check: func(t *testing.T, resp *querierv1.SelectMergeSpanProfileResponse) {
				require.Contains(t, resp.GetFlamegraph().GetNames(), "foo")
				require.Empty(t, resp.Tree)
			},
		},
		{
			name:   "pprof falls back to flamegraph",
			format: querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
			check: func(t *testing.T, resp *querierv1.SelectMergeSpanProfileResponse) {
				require.Contains(t, resp.GetFlamegraph().GetNames(), "foo")
				require.Empty(t, resp.Tree)
			},
		},
		{
			name:   "dot falls back to flamegraph",
			format: querierv1.ProfileFormat_PROFILE_FORMAT_DOT,
			check: func(t *testing.T, resp *querierv1.SelectMergeSpanProfileResponse) {
				require.Contains(t, resp.GetFlamegraph().GetNames(), "foo")
				require.Empty(t, resp.Tree)
			},
		},
		{
			name:   "tree",
			format: querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
			check: func(t *testing.T, resp *querierv1.SelectMergeSpanProfileResponse) {
				require.Equal(t, treeBytes, resp.Tree)
				require.Nil(t, resp.Flamegraph)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
			mockLimits.On("ForceDatasetIndexLookup", smpTenant).Return(false)
			mockLimits.On("MaxFlameGraphNodesDefault", smpTenant).Return(0)
			mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

			mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

			mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockBackend.On("Invoke", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					req := args.Get(1).(*queryv1.InvokeRequest)
					require.Len(t, req.Query, 1)
					require.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
					require.Equal(t, spanSelector, req.Query[0].Tree.GetSpanSelector())
				}).
				Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{Tree: treeBytes},
				}}}, nil)

			qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
			ctx := tenant.InjectTenantID(context.Background(), smpTenant)
			start, end := smpValidTimeRange()
			resp, err := qf.SelectMergeSpanProfile(ctx, connect.NewRequest(&querierv1.SelectMergeSpanProfileRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
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
