package queryfrontend

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
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockqueryfrontend"
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
				Labels:   []string{metadata.LabelNameUnsymbolized},
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
				Labels:   []string{metadata.LabelNameUnsymbolized, "__tenant_dataset__"},
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
				Labels:   []string{metadata.LabelNameUnsymbolized, "__tenant_dataset__"},
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
				Labels:   []string{metadata.LabelNameUnsymbolized, "__tenant_dataset__"},
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
				Labels:   []string{metadata.LabelNameUnsymbolized, "__tenant_dataset__"},
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
		hasUnsymbolized   bool
		setupMocks        func(*mockfrontend.MockLimits, *mockqueryfrontend.MockSymbolizer)
	}{
		{
			name:              "symbolization enabled for tenant with native profiles",
			tenantID:          "tenant1",
			symbolizerEnabled: true,
			hasUnsymbolized:   true,
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockqueryfrontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant1").Return(true)
				mockSymbolizer.On("SymbolizePprof", mock.Anything, mock.Anything).Return(nil).Once()
			},
		},
		{
			name:              "symbolization disabled for tenant",
			tenantID:          "tenant2",
			symbolizerEnabled: false,
			hasUnsymbolized:   true,
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockqueryfrontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant2").Return(false)
				mockSymbolizer.AssertNotCalled(t, "SymbolizePprof")
			},
		},
		{
			name:              "symbolization enabled but no native profiles",
			tenantID:          "tenant3",
			symbolizerEnabled: true,
			hasUnsymbolized:   false,
			setupMocks: func(mockLimits *mockfrontend.MockLimits, mockSymbolizer *mockqueryfrontend.MockSymbolizer) {
				mockLimits.On("SymbolizerEnabled", "tenant3").Return(true)
				mockSymbolizer.AssertNotCalled(t, "SymbolizePprof")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			mockSymbolizer := mockqueryfrontend.NewMockSymbolizer(t)
			tt.setupMocks(mockLimits, mockSymbolizer)

			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
				Reports: []*queryv1.Report{
					{
						Pprof: &queryv1.PprofReport{Pprof: createProfile(t)},
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

func createProfile(t *testing.T) []byte {
	t.Helper()

	stringTable := []string{
		"",
		"some_label",
		"some_value",
	}

	labels := []*profilev1.Label{{
		Key: 1,
		Str: 2,
	}}

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
