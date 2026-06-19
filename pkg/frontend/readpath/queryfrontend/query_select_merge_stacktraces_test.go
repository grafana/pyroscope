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
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

// TestSelectMergeStacktrace_Symbolization verifies that the frontend never
// symbolizes tree queries itself: symbolization is requested from the
// backend via InvokeOptions.Symbolize.
func TestSelectMergeStacktrace_Symbolization(t *testing.T) {
	tests := []struct {
		name              string
		tenantID          string
		symbolizerEnabled bool
	}{
		{
			name:              "symbolizer enabled for tenant",
			tenantID:          "tenant1",
			symbolizerEnabled: true,
		},
		{
			name:              "symbolizer disabled for tenant",
			tenantID:          "tenant2",
			symbolizerEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesDefault", tt.tenantID).Return(0)
			mockLimits.On("SymbolizerEnabled", tt.tenantID).Return(tt.symbolizerEnabled)
			mockLimits.On("QuerySanitizeOnMerge", tt.tenantID).Return(false)

			// Strict mock without expectations: any SymbolizePprof call fails.
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)

			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
				require.Len(t, req.Query, 1)
				require.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
				require.Equal(t, tt.symbolizerEnabled, req.Options.GetSymbolize())
				return true
			})).Return(&queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{},
				}},
			}, nil)

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
							"true",
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
			_, err := qf.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
			}))

			require.NoError(t, err)
			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}

func TestSelectMergeStacktraces_DotFormat(t *testing.T) {
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
	mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
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
	}))

	require.NoError(t, err)
	require.NotEmpty(t, resp.Msg.Dot)
	require.Contains(t, resp.Msg.Dot, "digraph")
	require.Nil(t, resp.Msg.Flamegraph)
	require.Empty(t, resp.Msg.Tree)
}
