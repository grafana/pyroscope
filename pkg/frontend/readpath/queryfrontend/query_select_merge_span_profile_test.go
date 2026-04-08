package queryfrontend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockqueryfrontend"
)

func TestSelectMergeSpanProfile_Symbolization(t *testing.T) {
	// pprofBytes contains a profile with one sample at location 1, where location 1
	// maps to function "original_func". backendTreeSymbolizer rewrites QUERY_TREE →
	// QUERY_PPROF before calling the upstream backend, so the mock must return this
	// pprof report. The symbolizer mock then renames the function to "symbolized_func",
	// which must be visible in the resulting flamegraph.
	pprofBytes := func() []byte {
		p := &profilev1.Profile{
			StringTable: []string{"", "original_func"},
			Function:    []*profilev1.Function{{Id: 1, Name: 1}},
			Location:    []*profilev1.Location{{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
			Mapping:     []*profilev1.Mapping{{Id: 1}},
			Sample:      []*profilev1.Sample{{LocationId: []uint64{1}, Value: []int64{10}}},
		}
		b, err := pprof.Marshal(p, true)
		require.NoError(t, err)
		return b
	}()

	spanSelector := []string{"span-abc123"}

	tests := []struct {
		name             string
		tenantID         string
		hasUnsymbolized  bool
		backendResp      *queryv1.InvokeResponse
		expectSymbolized bool
		setupMocks       func(*mockfrontend.MockLimits, *mockqueryfrontend.MockSymbolizer)
		checkInvokeReq   func(*testing.T, *queryv1.InvokeRequest)
	}{
		{
			name:             "symbolization enabled for tenant with native profiles",
			tenantID:         "tenant1",
			hasUnsymbolized:  true,
			expectSymbolized: true,
			// backendTreeSymbolizer rewrites QUERY_TREE -> QUERY_PPROF before calling
			// the upstream backend, so the mock must return a pprof report.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					Pprof: &queryv1.PprofReport{Pprof: pprofBytes},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant1").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant1").Return(false)
				s.On("SymbolizePprof", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						p := args.Get(1).(*profilev1.Profile)
						p.StringTable = append(p.StringTable, "symbolized_func")
						p.Function[0].Name = int64(len(p.StringTable) - 1)
					}).
					Return(nil).Once()
			},
			// backendTreeSymbolizer converts QUERY_TREE to QUERY_PPROF.
			// TODO: SpanSelector is not forwarded to PprofQuery (no span_selector field).
			checkInvokeReq: func(t *testing.T, req *queryv1.InvokeRequest) {
				require.Len(t, req.Query, 1)
				assert.Equal(t, queryv1.QueryType_QUERY_PPROF, req.Query[0].QueryType)
				assert.NotNil(t, req.Query[0].Pprof)
			},
		},
		{
			name:             "symbolization disabled for tenant",
			tenantID:         "tenant2",
			hasUnsymbolized:  true,
			expectSymbolized: false,
			// No backendTreeSymbolizer wrapping; backend receives the TREE query directly.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant2").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant2").Return(false)
			},
			// No symbolizer wrapping: backend receives QUERY_TREE with SpanSelector intact.
			checkInvokeReq: func(t *testing.T, req *queryv1.InvokeRequest) {
				require.Len(t, req.Query, 1)
				assert.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
				assert.Equal(t, spanSelector, req.Query[0].Tree.GetSpanSelector())
			},
		},
		{
			name:             "symbolization enabled but no native profiles",
			tenantID:         "tenant3",
			hasUnsymbolized:  false,
			expectSymbolized: false,
			// No backendTreeSymbolizer wrapping; backend receives the TREE query directly.
			backendResp: &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_TREE,
					Tree:       &queryv1.TreeReport{},
				}},
			},
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant3").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant3").Return(false)
			},
			// No symbolizer wrapping: backend receives QUERY_TREE with SpanSelector intact.
			checkInvokeReq: func(t *testing.T, req *queryv1.InvokeRequest) {
				require.Len(t, req.Query, 1)
				assert.Equal(t, queryv1.QueryType_QUERY_TREE, req.Query[0].QueryType)
				assert.Equal(t, spanSelector, req.Query[0].Tree.GetSpanSelector())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesDefault", tt.tenantID).Return(0)
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			checkInvokeReq := tt.checkInvokeReq
			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
				checkInvokeReq(t, req)
				return true
			})).Return(tt.backendResp, nil)

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
							fmt.Sprintf("%v", tt.hasUnsymbolized),
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
			resp, err := qf.SelectMergeSpanProfile(ctx, connect.NewRequest(&querierv1.SelectMergeSpanProfileRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
				SpanSelector:  spanSelector,
			}))

			require.NoError(t, err)
			names := resp.Msg.GetFlamegraph().GetNames()
			if tt.expectSymbolized {
				require.Contains(t, names, "symbolized_func")
				require.NotContains(t, names, "original_func")
			} else {
				require.NotContains(t, names, "symbolized_func")
			}

			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}
