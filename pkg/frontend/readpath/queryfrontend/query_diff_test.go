package queryfrontend

import (
	"context"
	"strings"
	"sync"
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

func TestDiff_ForwardsSpanSelectors(t *testing.T) {
	leftSpan := []string{"0000000000000001"}
	rightSpan := []string{"0000000000000002"}
	tree := new(phlaremodel.FunctionNameTree)
	tree.InsertStack(1, "foo")
	treeBytes := tree.Bytes(-1, nil)

	mockLimits := mockfrontend.NewMockLimits(t)
	mockLimits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	mockLimits.On("MaxFlameGraphNodesDefault", smpTenant).Return(0)
	mockLimits.On("QuerySanitizeOnMerge", smpTenant).Return(false)

	mockMetadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	mockMetadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)

	observed := make(map[string][]string)
	var observedMu sync.Mutex
	mockBackend := mockqueryfrontend.NewMockQueryBackend(t)
	mockBackend.On("Invoke", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			req := args.Get(1).(*queryv1.InvokeRequest)
			side := "right"
			if strings.Contains(req.LabelSelector, `side="left"`) {
				side = "left"
			}
			observedMu.Lock()
			observed[side] = req.Query[0].Tree.GetSpanSelector()
			observedMu.Unlock()
		}).
		Return(func(context.Context, *queryv1.InvokeRequest) *queryv1.InvokeResponse {
			return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_TREE,
				Tree:       &queryv1.TreeReport{Tree: treeBytes},
			}}}
		}, nil)

	qf := newSMPQueryFrontend(t, mockLimits, mockMetadata, mockBackend)
	ctx := tenant.InjectTenantID(context.Background(), smpTenant)
	start, end := smpValidTimeRange()
	resp, err := qf.Diff(ctx, connect.NewRequest(&querierv1.DiffRequest{
		Left: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: smpProfileType,
			LabelSelector: `{side="left"}`,
			Start:         start,
			End:           end,
			Format:        querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
			SpanSelector:  leftSpan,
		},
		Right: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: smpProfileType,
			LabelSelector: `{side="right"}`,
			Start:         start,
			End:           end,
			Format:        querierv1.ProfileFormat_PROFILE_FORMAT_DOT,
			SpanSelector:  rightSpan,
		},
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Flamegraph)
	require.Equal(t, leftSpan, observed["left"])
	require.Equal(t, rightSpan, observed["right"])
}
