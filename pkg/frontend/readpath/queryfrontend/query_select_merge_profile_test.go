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

func TestSelectMergeProfiles_Symbolization(t *testing.T) {
	pprofBytes := func() []byte {
		p := &profilev1.Profile{
			Sample: []*profilev1.Sample{{Value: []int64{10}}},
		}
		b, err := pprof.Marshal(p, true)
		require.NoError(t, err)
		return b
	}()

	backendResp := &queryv1.InvokeResponse{
		Reports: []*queryv1.Report{{
			ReportType: queryv1.ReportType_REPORT_PPROF,
			Pprof:      &queryv1.PprofReport{Pprof: pprofBytes},
		}},
	}

	tests := []struct {
		name             string
		tenantID         string
		hasUnsymbolized  bool
		expectSymbolized bool
		setupMocks       func(*mockfrontend.MockLimits, *mockqueryfrontend.MockSymbolizer)
	}{
		{
			name:             "symbolization enabled for tenant with unsymbolized profiles",
			tenantID:         "tenant1",
			hasUnsymbolized:  true,
			expectSymbolized: true,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant1").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant1").Return(false)
				s.On("SymbolizePprof", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						p := args.Get(1).(*profilev1.Profile)
						p.StringTable = append(p.StringTable, "symbolized")
					}).
					Return(nil).Once()
			},
		},
		{
			name:             "symbolization disabled for tenant",
			tenantID:         "tenant2",
			hasUnsymbolized:  true,
			expectSymbolized: false,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant2").Return(false)
				l.On("QuerySanitizeOnMerge", "tenant2").Return(false)
			},
		},
		{
			name:             "symbolization enabled but no unsymbolized profiles",
			tenantID:         "tenant3",
			hasUnsymbolized:  false,
			expectSymbolized: false,
			setupMocks: func(l *mockfrontend.MockLimits, s *mockqueryfrontend.MockSymbolizer) {
				l.On("SymbolizerEnabled", "tenant3").Return(true)
				l.On("QuerySanitizeOnMerge", "tenant3").Return(false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", tt.tenantID).Return(time.Duration(0))
			mockLimits.On("MaxFlameGraphNodesOnSelectMergeProfile", tt.tenantID).Return(false)
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(backendResp, nil)

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
			resp, err := qf.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
				ProfileTypeID: smpProfileType,
				LabelSelector: `{service_name="test-service"}`,
				Start:         start,
				End:           end,
			}))

			require.NoError(t, err)
			if tt.expectSymbolized {
				require.Contains(t, resp.Msg.StringTable, "symbolized")
			} else {
				require.NotContains(t, resp.Msg.StringTable, "symbolized")
			}

			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}
