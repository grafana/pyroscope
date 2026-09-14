package queryfrontend

import (
	"context"
	"fmt"
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

const fatCalleeTotal = 10000

// wideSandwich is a report whose callee half has one fat child and many thinner
// ones with distinct totals, so a node budget has something to cut. Totals must
// differ: the budget is a value threshold, as it is for tree truncation, so a
// half whose nodes all tie survives it intact.
func wideSandwich(children int) *querierv1.SandwichReport {
	names := []string{"f", "caller"}
	var thin int64
	callees := &querierv1.SandwichNode{NameIndex: 0}
	for i := 0; i < children; i++ {
		names = append(names, fmt.Sprintf("callee%04d", i))
		total := int64(i + 1)
		thin += total
		callees.Children = append(callees.Children, &querierv1.SandwichNode{
			NameIndex: int32(len(names) - 1), Total: total, Self: total,
		})
	}
	names = append(names, "fat")
	callees.Children = append(callees.Children, &querierv1.SandwichNode{
		NameIndex: int32(len(names) - 1), Total: fatCalleeTotal, Self: fatCalleeTotal,
	})
	total := thin + fatCalleeTotal
	callees.Total = total
	return &querierv1.SandwichReport{
		Names: names,
		Callers: &querierv1.SandwichNode{NameIndex: 0, Total: total, Children: []*querierv1.SandwichNode{
			{NameIndex: 1, Total: total},
		}},
		Callees: callees,
		Total:   total,
		Self:    3,
	}
}

func sandwichTotal(children int) int64 {
	return int64(children)*int64(children+1)/2 + fatCalleeTotal
}

func TestSelectSandwich_PassesFunctionAndSelectors(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
	limits.On("MaxFlameGraphNodesMax", smpTenant).Return(0)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	selector := &typesv1.StackTraceSelector{}
	backend.On("Invoke", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(1).(*queryv1.InvokeRequest)
		require.Equal(t, []string{smpTenant}, req.Tenant)
		require.Equal(t, queryv1.QueryType_QUERY_SANDWICH, req.Query[0].QueryType)
		require.Nil(t, req.Query[0].Tree)
		require.True(t, (&queryv1.SandwichQuery{
			Function: "runtime.mallocgc", StackTraceSelector: selector,
			ProfileIdSelector: []string{"profile-id"}, SpanSelector: []string{"0000000000000001"},
		}).EqualVT(req.Query[0].Sandwich))
	}).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_SANDWICH,
		Sandwich:   &queryv1.SandwichReport{Sandwich: wideSandwich(2)},
	}}}, nil)

	qf := newSMPQueryFrontend(t, limits, metadata, backend)
	start, end := smpValidTimeRange()
	maxNodes := int64(-1)
	fn := "runtime.mallocgc"
	resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_SANDWICH, SandwichFunction: &fn, MaxNodes: &maxNodes,
		StackTraceSelector: selector, ProfileIdSelector: []string{"profile-id"}, SpanSelector: []string{"0000000000000001"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Sandwich)
	require.Equal(t, sandwichTotal(2), resp.Msg.Sandwich.Total)
	require.Equal(t, "f", resp.Msg.Sandwich.Names[resp.Msg.Sandwich.Callers.NameIndex])
	require.Equal(t, "f", resp.Msg.Sandwich.Names[resp.Msg.Sandwich.Callees.NameIndex])
	require.Nil(t, resp.Msg.Flamegraph)
	require.Empty(t, resp.Msg.Tree)
}

func TestSelectSandwich_BudgetAppliedPerHalf(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("QuerySanitizeOnMerge", smpTenant).Return(false)
	limits.On("MaxFlameGraphNodesMax", smpTenant).Return(0)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(smpOneBlock(), nil)
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_SANDWICH,
		Sandwich:   &queryv1.SandwichReport{Sandwich: wideSandwich(500)},
	}}}, nil)

	qf := newSMPQueryFrontend(t, limits, metadata, backend)
	start, end := smpValidTimeRange()
	maxNodes := int64(3)
	fn := "f"
	resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_SANDWICH, SandwichFunction: &fn, MaxNodes: &maxNodes,
	}))
	require.NoError(t, err)
	s := resp.Msg.Sandwich

	// The callers half is small and survives untouched; the budget is not shared
	// with the callee half, which is where the cutting happens.
	require.Len(t, s.Callers.Children, 1)
	require.Less(t, len(s.Callees.Children), 501)

	byName := map[string]*querierv1.SandwichNode{}
	for _, c := range s.Callees.Children {
		byName[s.Names[c.NameIndex]] = c
	}
	require.Contains(t, byName, "fat", "the fat callee must survive any budget")
	require.Equal(t, int64(fatCalleeTotal), byName["fat"].Total)
	other, ok := byName["other"]
	require.True(t, ok, "the dropped callees need a stand-in")
	require.True(t, other.Truncated)

	// Truncation must not change what the profile says is in the function.
	want := sandwichTotal(500)
	require.Equal(t, want, s.Total)
	require.Equal(t, want, s.Callees.Total)
	var kept int64
	for _, c := range s.Callees.Children {
		kept += c.Total
	}
	require.Equal(t, want, kept, "kept children plus the stand-in must account for the whole half")
}

func TestSelectSandwich_FunctionRequired(t *testing.T) {
	qf := newSMPQueryFrontend(t, mockfrontend.NewMockLimits(t), new(mockmetastorev1.MockMetadataQueryServiceClient), mockqueryfrontend.NewMockQueryBackend(t))
	start, end := smpValidTimeRange()
	for _, fn := range []*string{nil, new(string)} {
		resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
			Format: querierv1.ProfileFormat_PROFILE_FORMAT_SANDWICH, SandwichFunction: fn,
		}))
		require.Nil(t, resp)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		require.ErrorContains(t, err, "sandwich_function is required")
	}
}

func TestSelectSandwich_Empty(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", smpTenant).Return(time.Duration(0))
	limits.On("MaxQueryLength", smpTenant).Return(time.Duration(0))
	limits.On("MaxFlameGraphNodesDefault", smpTenant).Return(16384)
	metadata := new(mockmetastorev1.MockMetadataQueryServiceClient)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{}, nil)
	qf := newSMPQueryFrontend(t, limits, metadata, mockqueryfrontend.NewMockQueryBackend(t))
	start, end := smpValidTimeRange()
	fn := "f"
	resp, err := qf.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), smpTenant), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: smpProfileType, LabelSelector: "{}", Start: start, End: end,
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_SANDWICH, SandwichFunction: &fn,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Sandwich)
	require.Zero(t, resp.Msg.Sandwich.Total)
	require.Empty(t, resp.Msg.Sandwich.Names)
}
