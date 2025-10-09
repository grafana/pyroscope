package queryfrontend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/featureflags"
	"github.com/grafana/pyroscope/pkg/pprof"
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
				mockLimits.On("QuerySanitizeOnMerge", "tenant1").Return(true)
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
				mockLimits.On("QuerySanitizeOnMerge", "tenant2").Return(true)
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
				mockLimits.On("QuerySanitizeOnMerge", "tenant3").Return(true)
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
			name:                "labels with dots pass through",
			allowUtf8LabelNames: false,
			setCapabilities:     true,
			backendLabelNames:   []string{"service.name", "app.version"},
			expectedLabelNames:  []string{"service.name", "app.version"},
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
				mockMetadataClient,
				nil,
				mockQueryBackend,
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
			name:                 "labels with dots pass through",
			allowUtf8LabelNames:  false,
			setCapabilities:      true,
			requestLabelNames:    []string{"service.name", "app.version"},
			expectedQueryRequest: []string{"service.name", "app.version"},
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
				mockMetadataClient,
				nil,
				mockQueryBackend,
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

func TestCreateStubsForUnsymbolizedProfiles(t *testing.T) {
	tests := []struct {
		name          string
		profile       *profilev1.Profile
		queries       []*queryv1.Query
		expectError   bool
		validateStubs func(t *testing.T, profile *profilev1.Profile)
	}{
		{
			name: "creates stubs for unsymbolized locations",
			profile: &profilev1.Profile{
				StringTable: []string{"", "/usr/lib/libjvm.so"},
				SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 1}},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 1, HasFunctions: false},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0xcafebabe, Line: nil},
					{Id: 2, MappingId: 1, Address: 0xdeadbeef, Line: nil},
				},
				Function: []*profilev1.Function{},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1, 2}, Value: []int64{100}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Location[0].Line, 1)
				require.Len(t, profile.Location[1].Line, 1)
				require.Len(t, profile.Function, 2)
				func1Name := profile.StringTable[profile.Function[0].Name]
				func2Name := profile.StringTable[profile.Function[1].Name]
				assert.Equal(t, "libjvm.so 0xcafebabe", func1Name)
				assert.Equal(t, "libjvm.so 0xdeadbeef", func2Name)
				assert.Equal(t, profile.Function[0].Id, profile.Location[0].Line[0].FunctionId)
				assert.Equal(t, profile.Function[1].Id, profile.Location[1].Line[0].FunctionId)
			},
		},
		{
			name: "deduplicates stubs by mapping and address combination",
			profile: &profilev1.Profile{
				StringTable: []string{"", "/lib/libc.so", "/lib/libm.so"},
				SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 1}},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 1, HasFunctions: false},
					{Id: 2, Filename: 2, HasFunctions: false},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0x1234, Line: nil},
					{Id: 2, MappingId: 1, Address: 0x5678, Line: nil},
					{Id: 3, MappingId: 2, Address: 0x1234, Line: nil},
				},
				Function: []*profilev1.Function{},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1, 2, 3}, Value: []int64{50}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Function, 3)
				func1Name := profile.StringTable[profile.Function[0].Name]
				func2Name := profile.StringTable[profile.Function[1].Name]
				func3Name := profile.StringTable[profile.Function[2].Name]
				assert.Equal(t, "libc.so 0x1234", func1Name)
				assert.Equal(t, "libc.so 0x5678", func2Name)
				assert.Equal(t, "libm.so 0x1234", func3Name)
			},
		},
		{
			name: "skips already symbolized locations",
			profile: &profilev1.Profile{
				StringTable: []string{"", "/usr/bin/app", "symbolized_func"},
				SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 1}},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 1, HasFunctions: true},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0x1000, Line: []*profilev1.Line{{FunctionId: 1}}},
					{Id: 2, MappingId: 1, Address: 0x2000, Line: nil},
				},
				Function: []*profilev1.Function{
					{Id: 1, Name: 2},
				},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1, 2}, Value: []int64{25}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Location[0].Line, 1)
				assert.Equal(t, uint64(1), profile.Location[0].Line[0].FunctionId)
				require.Len(t, profile.Location[1].Line, 1)
				require.Len(t, profile.Function, 2)
				stubFunc := profile.Function[1]
				stubName := profile.StringTable[stubFunc.Name]
				assert.Equal(t, "app 0x2000", stubName)
			},
		},
		{
			name: "handles multiple mappings",
			profile: &profilev1.Profile{
				StringTable: []string{"", "/lib/liba.so", "/lib/libb.so"},
				SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 1}},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 1, HasFunctions: false},
					{Id: 2, Filename: 2, HasFunctions: false},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0x100, Line: nil},
					{Id: 2, MappingId: 2, Address: 0x200, Line: nil},
				},
				Function: []*profilev1.Function{},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1, 2}, Value: []int64{10}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Function, 2)
				func1Name := profile.StringTable[profile.Function[0].Name]
				func2Name := profile.StringTable[profile.Function[1].Name]
				assert.Equal(t, "liba.so 0x100", func1Name)
				assert.Equal(t, "libb.so 0x200", func2Name)
			},
		},
		{
			name: "handles unknown binary name",
			profile: &profilev1.Profile{
				StringTable: []string{""},
				SampleType:  []*profilev1.ValueType{{Type: 0, Unit: 0}},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 0, HasFunctions: false},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0xabc, Line: nil},
				},
				Function: []*profilev1.Function{},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{5}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Function, 1)
				funcName := profile.StringTable[profile.Function[0].Name]
				assert.Equal(t, "unknown 0xabc", funcName)
			},
		},
		{
			name: "query/report count mismatch returns error",
			profile: &profilev1.Profile{
				StringTable: []string{""},
				Location:    []*profilev1.Location{},
				Function:    []*profilev1.Function{},
				Sample:      []*profilev1.Sample{},
			},
			queries: []*queryv1.Query{
				{QueryType: queryv1.QueryType_QUERY_PPROF},
				{QueryType: queryv1.QueryType_QUERY_PPROF},
			},
			expectError: true,
		},
		{
			name: "no changes needed returns early",
			profile: &profilev1.Profile{
				StringTable: []string{"", "/usr/bin/app", "func1"},
				Mapping: []*profilev1.Mapping{
					{Id: 1, Filename: 1, HasFunctions: true},
				},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Address: 0x1000, Line: []*profilev1.Line{{FunctionId: 1}}},
				},
				Function: []*profilev1.Function{
					{Id: 1, Name: 2},
				},
				Sample: []*profilev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{10}},
				},
			},
			queries: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_PPROF}},
			validateStubs: func(t *testing.T, profile *profilev1.Profile) {
				require.Len(t, profile.Function, 1, "should not add functions")
				assert.Equal(t, "func1", profile.StringTable[2], "should not modify string table")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileBytes, err := pprof.Marshal(tt.profile, true)
			require.NoError(t, err)

			resp := &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{
					{
						Pprof:      &queryv1.PprofReport{Pprof: profileBytes},
						ReportType: queryv1.ReportType_REPORT_PPROF,
					},
				},
			}

			qf := &QueryFrontend{}
			err = qf.createStubsForUnsymbolizedProfiles(context.Background(), resp, tt.queries)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			var resultProfile profilev1.Profile
			err = pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &resultProfile)
			require.NoError(t, err)

			if tt.validateStubs != nil {
				tt.validateStubs(t, &resultProfile)
			}
		})
	}
}
