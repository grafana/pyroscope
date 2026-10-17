package queryfrontend

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/featureflags"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

const (
	smpProfileType = "memory:inuse_space:bytes:space:byte"
)

func smpValidTimeRange() (int64, int64) {
	now := time.Now().UnixMilli()
	return now, now + time.Minute.Milliseconds()
}

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

func Test_QueryFrontend_LabelNames_WithFiltering(t *testing.T) {
	tests := []struct {
		name                string
		allowUtf8LabelNames bool
		setCapabilities     bool
		backendLabelNames   []string
		expectedLabelNames  []string
	}{
		{
			name:                "UTF8 labels allowed when enabled",
			allowUtf8LabelNames: true,
			setCapabilities:     true,
			backendLabelNames:   []string{"foo", "bar", "世界"},
			expectedLabelNames:  []string{"foo", "bar", "世界"},
		},
		{
			name:                "UTF8 labels filtered when disabled",
			allowUtf8LabelNames: false,
			setCapabilities:     true,
			backendLabelNames:   []string{"foo", "bar", "世界"},
			expectedLabelNames:  []string{"foo", "bar"},
		},
		{
			name:                "invalid labels pass through when UTF8 enabled",
			allowUtf8LabelNames: true,
			setCapabilities:     true,
			backendLabelNames:   []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
			expectedLabelNames:  []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
		},
		{
			name:                "invalid labels filtered when UTF8 disabled",
			allowUtf8LabelNames: false,
			setCapabilities:     true,
			backendLabelNames:   []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
			expectedLabelNames:  []string{"valid_name"},
		},
		{
			name:               "filtering enabled when no capabilities set",
			setCapabilities:    false,
			backendLabelNames:  []string{"valid_name", "123invalid", "世界"},
			expectedLabelNames: []string{"valid_name"},
		},
		{
			name:                "labels with dots do not pass through",
			allowUtf8LabelNames: false,
			setCapabilities:     true,
			backendLabelNames:   []string{"service.name", "app.version"},
			expectedLabelNames:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)
			mockQueryBackend.On("Invoke", mock.Anything, mock.Anything).Return(&queryv1.InvokeResponse{
				Reports: []*queryv1.Report{
					{
						ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
						LabelNames: &queryv1.LabelNamesReport{
							LabelNames: tc.backendLabelNames,
						},
					},
				},
			}, nil)

			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", "test-tenant").Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", "test-tenant").Return(time.Duration(0))
			mockLimits.On("QuerySanitizeOnMerge", "test-tenant").Return(true)
			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "test-block"}},
			}, nil)

			qf := NewQueryFrontend(
				log.NewNopLogger(),
				mockLimits,
				frontend.Config{},
				mockMetadataClient,
				nil,
				mockQueryBackend,
				nil,
				nil,
				nil,
			)

			ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
			if tc.setCapabilities {
				ctx = featureflags.WithClientCapabilities(ctx, featureflags.ClientCapabilities{
					AllowUtf8LabelNames: tc.allowUtf8LabelNames,
				})
			}

			req := connect.NewRequest(&typesv1.LabelNamesRequest{
				Start: 1000,
				End:   2000,
			})

			resp, err := qf.LabelNames(ctx, req)
			require.NoError(t, err)
			require.Equal(t, tc.expectedLabelNames, resp.Msg.Names)
		})
	}
}

func Test_QueryFrontend_Series_WithLabelNameFiltering(t *testing.T) {
	tests := []struct {
		name                 string
		allowUtf8LabelNames  bool
		setCapabilities      bool
		requestLabelNames    []string
		backendLabelNames    []string // For empty request case
		expectedQueryRequest []string // What should be passed to backend
	}{
		{
			name:                 "all label names pass through when UTF8 enabled",
			allowUtf8LabelNames:  true,
			setCapabilities:      true,
			requestLabelNames:    []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
			expectedQueryRequest: []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
		},
		{
			name:                 "invalid label names filtered when UTF8 disabled",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{"valid_name", "123invalid", "invalid-hyphen", "世界"},
			expectedQueryRequest: []string{"valid_name"},
		},
		{
			name:                 "UTF8 labels filtered when UTF8 disabled",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{"foo", "bar", "世界", "日本語"},
			expectedQueryRequest: []string{"foo", "bar"},
		},
		{
			name:                 "filtering enabled when no capabilities set",
			setCapabilities:      false,
			requestLabelNames:    []string{"foo", "123invalid", "世界"},
			expectedQueryRequest: []string{"foo"},
		},
		{
			name:                 "all valid labels pass through",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{"foo", "bar", "service_name"},
			expectedQueryRequest: []string{"foo", "bar", "service_name"},
		},
		{
			name:                 "labels with dots do not pass through",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{"service.name", "app.version"},
			expectedQueryRequest: []string{},
		},
		{
			name:                 "empty label names with UTF8 disabled queries and filters all labels",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{},
			backendLabelNames:    []string{"foo", "bar", "世界"},
			expectedQueryRequest: []string{"foo", "bar"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLabelNames []string

			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)

			// For empty label names case, we need to mock the LabelNames query first
			if len(tc.requestLabelNames) == 0 {
				mockQueryBackend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
					return len(req.Query) > 0 && req.Query[0].QueryType == queryv1.QueryType_QUERY_LABEL_NAMES
				})).Return(&queryv1.InvokeResponse{
					Reports: []*queryv1.Report{
						{
							ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
							LabelNames: &queryv1.LabelNamesReport{
								LabelNames: tc.backendLabelNames,
							},
						},
					},
				}, nil).Once()
			}

			// Mock the Series query specifically
			mockQueryBackend.On("Invoke", mock.Anything, mock.MatchedBy(func(req *queryv1.InvokeRequest) bool {
				return len(req.Query) > 0 && req.Query[0].QueryType == queryv1.QueryType_QUERY_SERIES_LABELS
			})).Run(func(args mock.Arguments) {
				invReq := args.Get(1).(*queryv1.InvokeRequest)
				if len(invReq.Query) > 0 && invReq.Query[0].SeriesLabels != nil {
					capturedLabelNames = invReq.Query[0].SeriesLabels.LabelNames
					if capturedLabelNames == nil {
						capturedLabelNames = []string{}
					}
				}
			}).Return(&queryv1.InvokeResponse{
				Reports: []*queryv1.Report{
					{
						ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
						SeriesLabels: &queryv1.SeriesLabelsReport{
							SeriesLabels: []*typesv1.Labels{},
						},
					},
				},
			}, nil).Once()

			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", "test-tenant").Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", "test-tenant").Return(time.Duration(0))
			mockLimits.On("QuerySanitizeOnMerge", "test-tenant").Return(true)
			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadataClient.On("QueryMetadata", mock.Anything, mock.Anything).Return(&metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "test-block"}},
			}, nil)

			qf := NewQueryFrontend(
				log.NewNopLogger(),
				mockLimits,
				frontend.Config{},
				mockMetadataClient,
				nil,
				mockQueryBackend,
				nil,
				nil,
				nil,
			)

			ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
			if tc.setCapabilities {
				ctx = featureflags.WithClientCapabilities(ctx, featureflags.ClientCapabilities{
					AllowUtf8LabelNames: tc.allowUtf8LabelNames,
				})
			}

			req := connect.NewRequest(&querierv1.SeriesRequest{
				Matchers:   []string{`{service_name="test"}`},
				LabelNames: tc.requestLabelNames,
				Start:      1000,
				End:        2000,
			})

			_, err := qf.Series(ctx, req)
			require.NoError(t, err)

			// Verify that the label names were filtered correctly before being sent to backend
			require.Equal(t, tc.expectedQueryRequest, capturedLabelNames,
				"Expected label names sent to backend to be %v, but got %v", tc.expectedQueryRequest, capturedLabelNames)
		})
	}
}

func TestQueryFrontend_BackendInflightWork(t *testing.T) {
	reg := prometheus.NewRegistry()
	blocks := []*metastorev1.BlockMeta{
		{Id: "a", Datasets: []*metastorev1.Dataset{{Size: 100, TableOfContents: []uint64{0, 20, 60}}}},
		{Id: "b", Datasets: []*metastorev1.Dataset{{Size: 250, TableOfContents: []uint64{0, 50, 100}}}},
	}
	backend := mockqueryfrontend.NewMockQueryBackend(t)
	backend.On("Invoke", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
		require.Equal(t, float64(2), backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_blocks"))
		require.Equal(t, float64(350), backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_estimated_bytes"))
	}).Return(&queryv1.InvokeResponse{}, nil).Once()
	qf := backendWorkFrontend(t, reg, blocks, backend)

	ctx := tenant.InjectTenantID(t.Context(), "test-tenant")
	_, err := qf.Query(ctx, &queryv1.QueryRequest{LabelSelector: "{}"})
	require.NoError(t, err)
	require.Zero(t, backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_blocks"))
	require.Zero(t, backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_estimated_bytes"))
}

func TestQueryFrontend_BackendInflightWorkClearedOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancel    bool
		returnErr error
	}{
		{name: "backend error", returnErr: errors.New("backend failed")},
		{name: "cancellation", cancel: true, returnErr: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			backend := mockqueryfrontend.NewMockQueryBackend(t)
			qf := backendWorkFrontend(t, reg, []*metastorev1.BlockMeta{
				{Id: "a", Datasets: []*metastorev1.Dataset{{Size: 100, TableOfContents: []uint64{0, 20, 60}}}},
			}, backend)

			ctx, cancel := context.WithCancel(tenant.InjectTenantID(t.Context(), "test-tenant"))
			defer cancel()
			backend.On("Invoke", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
				require.Equal(t, float64(1), backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_blocks"))
				require.Equal(t, float64(100), backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_estimated_bytes"))
				if tc.cancel {
					cancel()
				}
			}).Return(nil, func(ctx context.Context, _ *queryv1.InvokeRequest) error {
				if tc.cancel {
					return ctx.Err()
				}
				return tc.returnErr
			}).Once()

			_, err := qf.Query(ctx, &queryv1.QueryRequest{LabelSelector: "{}"})
			require.ErrorIs(t, err, tc.returnErr)
			require.Zero(t, backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_blocks"))
			require.Zero(t, backendWorkGauge(t, reg, "pyroscope_query_frontend_backend_inflight_estimated_bytes"))
		})
	}
}

func backendWorkFrontend(t *testing.T, reg *prometheus.Registry, blocks []*metastorev1.BlockMeta, backend QueryBackend) *QueryFrontend {
	t.Helper()
	metadata := mockmetastorev1.NewMockMetadataQueryServiceClient(t)
	metadata.On("QueryMetadata", mock.Anything, mock.Anything).Return(
		func(context.Context, *metastorev1.QueryMetadataRequest, ...grpc.CallOption) *metastorev1.QueryMetadataResponse {
			return &metastorev1.QueryMetadataResponse{Blocks: slices.Clone(blocks)}
		}, nil)
	limits := mockfrontend.NewMockLimits(t)
	limits.On("QuerySanitizeOnMerge", "test-tenant").Return(false)
	return NewQueryFrontend(log.NewNopLogger(), limits, frontend.Config{}, metadata, nil, backend, nil, nil, reg)
}

func backendWorkGauge(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			require.Len(t, family.GetMetric(), 1)
			require.Empty(t, family.GetMetric()[0].GetLabel())
			return family.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}
