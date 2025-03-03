package symbolizer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	pprof "github.com/google/pprof/profile"
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	mocksymbolizer "github.com/grafana/pyroscope/pkg/test/mocks/mocksymbolizer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestSymbolizePprof tests symbolization using testdata/symbols.debug which contains:
//
// 0x1500 ->
//
//	 main (/usr/src/stress-1.0.7-1/src/stress.c:87)
//		fprintf (/usr/include/x86_64-linux-gnu/bits/stdio2.h:77)
//
// 0x3c5a -> atoll_b (/usr/src/stress-1.0.7-1/src/stress.c:632)
// 0x2745 -> main (/usr/src/stress-1.0.7-1/src/stress.c:87)
func TestSymbolizePprof(t *testing.T) {
	tests := []struct {
		name      string
		profile   *googlev1.Profile
		setupMock func(*mocksymbolizer.MockDebuginfodClient)
		wantErr   bool
		validate  func(*testing.T, *googlev1.Profile)
	}{
		{
			name: "already symbolized mapping",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					HasFunctions:   true,
					HasFilenames:   true,
					HasLineNumbers: true,
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Line: []*googlev1.Line{{
						FunctionId: 0,
						Line:       42,
					}},
				}},
				Function: []*googlev1.Function{{
					Name:     1,
					Filename: 2,
				}},
				StringTable: []string{"", "main", "main.go"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)
				require.True(t, p.Mapping[0].HasFilenames)
				require.True(t, p.Mapping[0].HasLineNumbers)
			},
		},
		{
			name: "needs symbolization",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:     1,
					MemoryStart: 0x0,
					MemoryLimit: 0x1000000,
					FileOffset:  0x0,
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Address:   0x1500,
				}},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)
				require.True(t, p.Mapping[0].HasFilenames)
				require.True(t, p.Mapping[0].HasLineNumbers)

				// Validate first location has two lines (main and fprintf)
				require.Len(t, p.Location[0].Line, 2)

				// Check main function
				mainFunc := p.Function[p.Location[0].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc.Name])
				require.Equal(t, "/usr/src/stress-1.0.7-1/src/stress.c", p.StringTable[mainFunc.Filename])
				require.Equal(t, int64(87), p.Location[0].Line[0].Line)
				require.Equal(t, int64(86), mainFunc.StartLine)

				// Check fprintf function
				fprintfFunc := p.Function[p.Location[0].Line[1].FunctionId]
				require.Equal(t, "fprintf", p.StringTable[fprintfFunc.Name])
				require.Equal(t, "/usr/include/x86_64-linux-gnu/bits/stdio2.h", p.StringTable[fprintfFunc.Filename])
				require.Equal(t, int64(77), p.Location[0].Line[1].Line)
				require.Equal(t, int64(77), fprintfFunc.StartLine)
			},
		},
		{
			name: "invalid function references",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:      1,
					HasFunctions: true, // Incorrectly set
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Line: []*googlev1.Line{{
						FunctionId: 999, // Invalid reference
					}},
				}},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Maybe()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				// Should detect invalid function reference and fix mapping flags
				require.False(t, p.Mapping[0].HasFunctions)
			},
		},
		{
			name: "empty build ID",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId: 1,
				}},
				StringTable: []string{"", ""},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.False(t, p.Mapping[0].HasFunctions)
			},
		},
		{
			name: "multiple locations per mapping",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:     1,
					MemoryStart: 0x0,
					MemoryLimit: 0x1000000,
					FileOffset:  0x0,
				}},
				Location: []*googlev1.Location{
					{MappingId: 1, Address: 0x1500},
					{MappingId: 1, Address: 0x3c5a},
					{MappingId: 1, Address: 0x2745},
				},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)

				// First location (0x1500) - main and fprintf
				require.Len(t, p.Location[0].Line, 2)
				mainFunc := p.Function[p.Location[0].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc.Name])

				// Second location (0x3c5a) - atoll_b
				require.Len(t, p.Location[1].Line, 1)
				atollFunc := p.Function[p.Location[1].Line[0].FunctionId]
				require.Equal(t, "atoll_b", p.StringTable[atollFunc.Name])
				require.Equal(t, int64(632), p.Location[1].Line[0].Line)

				// Third location (0x2745) - main
				require.Len(t, p.Location[2].Line, 1)
				mainFunc2 := p.Function[p.Location[2].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc2.Name])
				require.Equal(t, int64(87), p.Location[2].Line[0].Line)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient)

			s := NewProfileSymbolizer(mockClient, NewNullCache(), NewMetrics(nil), 0)

			err := s.SymbolizePprof(context.Background(), tt.profile)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			tt.validate(t, tt.profile)
			mockClient.AssertExpectations(t)
		})
	}
}

func TestSymbolizerMetrics(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mocksymbolizer.MockDebuginfodClient)
		setupTest   func(*ProfileSymbolizer, context.Context) error
		expected    string
		metricNames []string
	}{
		{
			name: "successful symbolization",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			setupTest: func(s *ProfileSymbolizer, ctx context.Context) error {
				profile := &googlev1.Profile{
					Mapping: []*googlev1.Mapping{{
						BuildId:     1,
						MemoryStart: 0x0,
						MemoryLimit: 0x1000000,
					}},
					Location: []*googlev1.Location{{
						MappingId: 1,
						Address:   0x1500,
					}},
					StringTable: []string{"", "build-id"},
				}
				return s.SymbolizePprof(ctx, profile)
			},
			expected: `
				# HELP pyroscope_profile_symbolization_total Total number of profiles processed for symbolization
				# TYPE pyroscope_profile_symbolization_total counter
				pyroscope_profile_symbolization_total 1

				# HELP pyroscope_debug_symbol_resolutions_total Total number of debug symbol resolutions attempted by status
				# TYPE pyroscope_debug_symbol_resolutions_total counter
				pyroscope_debug_symbol_resolutions_total{status="success"} 1

		    `,
			metricNames: []string{
				"pyroscope_profile_symbolization_total",
				"pyroscope_debug_symbol_resolutions_total",
			},
		},
		{
			name: "debuginfod error",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "unknown-build-id").
					Return(nil, fmt.Errorf("unknown build ID")).Once()
			},
			setupTest: func(s *ProfileSymbolizer, ctx context.Context) error {
				profile := &googlev1.Profile{
					Mapping: []*googlev1.Mapping{{
						BuildId: 1,
					}},
					Location: []*googlev1.Location{{
						MappingId: 1,
						Address:   0x1500,
					}},
					StringTable: []string{"", "unknown-build-id"},
				}
				return s.SymbolizePprof(ctx, profile)
			},
			expected: `
				# HELP pyroscope_profile_symbolization_errors_total Total number of profile symbolization errors
				# TYPE pyroscope_profile_symbolization_errors_total counter
				pyroscope_profile_symbolization_errors_total{reason="symbolization_error"} 1

				# HELP pyroscope_debug_symbol_resolution_errors_total Total number of debug symbol resolution errors by reason
				# TYPE pyroscope_debug_symbol_resolution_errors_total counter
				pyroscope_debug_symbol_resolution_errors_total{reason="debuginfod_error"} 1
			`,
			metricNames: []string{
				"pyroscope_profile_symbolization_errors_total",
				"pyroscope_debug_symbol_resolution_errors_total",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			metrics := NewMetrics(reg)

			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient)

			s := NewProfileSymbolizer(mockClient, NewNullCache(), metrics, 0)

			err := tt.setupTest(s, context.Background())
			if err != nil {
				t.Log("Setup error:", err)
			}

			err = testutil.GatherAndCompare(reg, strings.NewReader(tt.expected), tt.metricNames...)
			require.NoError(t, err)

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSymbolizeWithCache(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)

	// First call should return the test file
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Create a symbolizer with LRU cache
	s := NewProfileSymbolizer(mockClient, NewNullCache(), metrics, 100)

	// First request - should be a cache miss
	req1 := &Request{
		BuildID: "build-id",
		Locations: []*Location{
			{
				Address: 0x1500,
				Mapping: &pprof.Mapping{
					Start:   0x0,
					Limit:   0x1000000,
					Offset:  0x0,
					BuildID: "build-id",
				},
			},
		},
	}
	err := s.Symbolize(context.Background(), req1)
	require.NoError(t, err)
	require.NotEmpty(t, req1.Locations[0].Lines)

	time.Sleep(10 * time.Millisecond)

	// Second request with same address - should be a cache hit
	req2 := &Request{
		BuildID: "build-id",
		Locations: []*Location{
			{
				Address: 0x1500,
				Mapping: &pprof.Mapping{
					Start:   0x0,
					Limit:   0x1000000,
					Offset:  0x0,
					BuildID: "build-id",
				},
			},
		},
	}

	// No additional calls expected for the second request

	err = s.Symbolize(context.Background(), req2)
	require.NoError(t, err)
	require.NotEmpty(t, req2.Locations[0].Lines)

	// Third request with different address - should be a cache miss
	req3 := &Request{
		BuildID: "build-id",
		Locations: []*Location{
			{
				Address: 0x3c5a,
				Mapping: &pprof.Mapping{
					Start:   0x0,
					Limit:   0x1000000,
					Offset:  0x0,
					BuildID: "build-id",
				},
			},
		},
	}

	// Should call FetchDebuginfo again for the new address
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()

	err = s.Symbolize(context.Background(), req3)
	require.NoError(t, err)
	require.NotEmpty(t, req3.Locations[0].Lines)

	mockClient.AssertExpectations(t)
}

// Helper function to open the test file
func openTestFile(t *testing.T) io.ReadCloser {
	t.Helper()
	f, err := os.Open("testdata/symbols.debug")
	require.NoError(t, err)
	return f
}
