package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

// TestSelectMergeSpanProfile_Symbolization verifies that span-filtered tree
// queries reach the backend with the span selector intact and that
// symbolization is delegated via InvokeOptions.Symbolize.
func TestSelectMergeSpanProfile_Symbolization(t *testing.T) {
	spanSelector := []string{"span-abc123"}

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
				assert.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
				assert.Equal(t, spanSelector, req.Query[0].Tree.GetSpanSelector())
				assert.Equal(t, tt.symbolizerEnabled, req.Options.GetSymbolize())
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
			)

			ctx := tenant.InjectTenantID(context.Background(), tt.tenantID)
			start, end := smpValidTimeRange()
			_, err := qf.SelectMergeSpanProfile(ctx, connect.NewRequest(&querierv1.SelectMergeSpanProfileRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
				SpanSelector:  spanSelector,
			}))

			require.NoError(t, err)
			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}
