package query_frontend

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockquery_frontend"
)

func Test_QueryFrontend_QueryMetadata(t *testing.T) {
	for _, test := range []struct {
		query    *queryv1.QueryRequest
		request  *metastorev1.QueryMetadataRequest
		response *metastorev1.QueryMetadataResponse
	}{
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name="service-a"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{service_name="service-a"}`,
				Labels:   []string{"__has_native_profiles__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_a"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name!="service-a"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__has_native_profiles__", "__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_a"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name=~".*"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__has_native_profiles__", "__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_c"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{foo="bar"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__has_native_profiles__", "__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_b"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: "{}"},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__has_native_profiles__", "__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_d"}},
			},
		},
	} {
		mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
		ctx := user.InjectOrgID(context.Background(), "org")
		f := &QueryFrontend{metadataQueryClient: mockMetadataClient}

		mockMetadataClient.On("QueryMetadata", mock.Anything, test.request).
			Return(test.response, nil).
			Once()

		blocks, err := f.QueryMetadata(ctx, test.query)
		assert.NoError(t, err)
		assert.Equal(t, test.response.Blocks, blocks)
	}
}

func TestQueryFrontendSymbolization(t *testing.T) {
	tests := []struct {
		name              string
		tenantID          string
		symbolizerEnabled bool
		hasNativeProfiles bool
		profileType       string
		setupMocks        func(*mockfrontend.MockLimits, *mockquery_frontend.MockSymbolizer)
	}{
		{
			name:              "symbolization enabled for tenant with native profiles",
			tenantID:          "tenant1",
			symbolizerEnabled: true,
			hasNativeProfiles: true,
			profileType:       "otel",
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockquery_frontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant1").Return(true)
				mockSymbolizer.On("SymbolizePprof", mock.Anything, mock.Anything).Return(nil).Once()
			},
		},
		{
			name:              "symbolization disabled for tenant",
			tenantID:          "tenant2",
			symbolizerEnabled: false,
			hasNativeProfiles: true,
			profileType:       "otel",
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockquery_frontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant2").Return(false)
				mockSymbolizer.AssertNotCalled(t, "SymbolizePprof")
			},
		},
		{
			name:              "symbolization enabled but no native profiles",
			tenantID:          "tenant3",
			symbolizerEnabled: true,
			hasNativeProfiles: false,
			profileType:       "otel",
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockquery_frontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant3").Return(true)
				mockSymbolizer.AssertNotCalled(t, "SymbolizePprof")
			},
		},
		{
			name:              "symbolization enabled but non-OTEL profile",
			tenantID:          "tenant4",
			symbolizerEnabled: true,
			hasNativeProfiles: true,
			profileType:       "non-otel",
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockquery_frontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant4").Return(true)
				mockSymbolizer.AssertNotCalled(t, "SymbolizePprof")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockSymbolizer := mockquery_frontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			mockQueryBackend := mockquery_frontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
				Reports: []*queryv1.Report{
					{
						Pprof: &queryv1.PprofReport{Pprof: createProfile(t, tt.profileType)},
					},
				},
			}, nil)

			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).
				Return(&metastorev1.QueryMetadataResponse{
					Blocks: []*metastorev1.BlockMeta{{
						Id: "block_id_d",
						Datasets: []*metastorev1.Dataset{{
							Labels: []int32{1, 1, 2},
						}},
						StringTable: []string{
							"", // First string is always empty by convention
							metadata.LabelNameHasNativeProfiles,
							fmt.Sprintf("%v", tt.hasNativeProfiles),
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
			)

			ctx := tenant.InjectTenantID(context.Background(), tt.tenantID)
			_, err := qf.Query(ctx, &queryv1.QueryRequest{
				LabelSelector: `{service_name="test-service"}`,
				Query: []*queryv1.Query{
					{
						QueryType: queryv1.QueryType_QUERY_PPROF,
					},
				},
			})

			require.NoError(t, err)

			mockMetadataClient.AssertExpectations(t)
			mockQueryBackend.AssertExpectations(t)
		})
	}
}

func createProfile(t *testing.T, profileType string) []byte {
	t.Helper()

	var stringTable []string
	var labels []*profilev1.Label

	switch profileType {
	case "otel":
		stringTable = []string{
			"",                        // Index 0 is always empty
			phlaremodel.LabelNameOTEL, // Index 1
			"true",                    // Index 2
		}
		labels = []*profilev1.Label{{
			Key: 1, // Index of "__otel__"
			Str: 2, // Index of "true"
		}}
	case "non-otel":
		stringTable = []string{
			"",           // Index 0 is always empty
			"some_label", // Index 1
			"some_value", // Index 2
		}
		labels = []*profilev1.Label{{
			Key: 1,
			Str: 2,
		}}
	default:
		t.Fatalf("unknown profile type: %s", profileType)
	}

	profile := &profilev1.Profile{
		StringTable: stringTable,
		Sample: []*profilev1.Sample{{
			Label: labels,
		}},
	}

	bytes, err := profile.MarshalVT()
	require.NoError(t, err)
	return bytes
}
