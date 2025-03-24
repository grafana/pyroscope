package symbolizer

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/go-kit/log"
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
// 0x1500 -> (contains both functions)
//   - main (/usr/src/stress-1.0.7-1/src/stress.c:87)
//   - fprintf (/usr/include/x86_64-linux-gnu/bits/stdio2.h:77)
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
				mainFunc := assertLocationHasFunction(t, p, p.Location[0], "main",
					"/usr/src/stress-1.0.7-1/src/stress.c", 87)
				require.Equal(t, int64(86), mainFunc.StartLine, "main function start line mismatch")

				// Check fprintf function
				fprintfFunc := assertLocationHasFunction(t, p, p.Location[0], "fprintf",
					"/usr/include/x86_64-linux-gnu/bits/stdio2.h", 77)
				require.Equal(t, int64(77), fprintfFunc.StartLine, "fprintf function start line mismatch")
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
				// With the new approach, the test should verify that the invalid location gets symbolized
				// instead of checking if mapping flags are reset
				require.True(t, p.Mapping[0].HasFunctions, "Mapping flags should stay unchanged")
				// Verify that we added proper symbols to the location
				require.NotEmpty(t, p.Location[0].Line)
				require.Equal(t, p.Location[0].Line[0].FunctionId, uint64(1))
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
				assertLocationHasFunction(t, p, p.Location[0], "main",
					"/usr/src/stress-1.0.7-1/src/stress.c", 87)
				assertLocationHasFunction(t, p, p.Location[0], "fprintf",
					"/usr/include/x86_64-linux-gnu/bits/stdio2.h", 77)

				// Second location (0x3c5a) - atoll_b
				require.Len(t, p.Location[1].Line, 1)
				assertLocationHasFunction(t, p, p.Location[1], "atoll_b",
					"/usr/src/stress-1.0.7-1/src/stress.c", 632)

				// Third location (0x2745) - main
				require.Len(t, p.Location[2].Line, 1)
				assertLocationHasFunction(t, p, p.Location[2], "main",
					"/usr/src/stress-1.0.7-1/src/stress.c", 87)
			},
		},
		{
			name: "mixed symbolization - preserves valid symbols",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{
					{ // First mapping - already symbolized
						BuildId:        1, // "already-symbolized-id" in string table
						HasFunctions:   true,
						HasFilenames:   true,
						HasLineNumbers: true,
						MemoryStart:    0x0,
						MemoryLimit:    0x1000,
						FileOffset:     0x0,
						Filename:       2, // "lib1.so" in string table
					},
					{ // Second mapping - needs symbolization
						BuildId:        3, // "build-id" in string table
						HasFunctions:   false,
						HasFilenames:   false,
						HasLineNumbers: false,
						MemoryStart:    0x1000,
						MemoryLimit:    0x2000,
						FileOffset:     0x0,
						Filename:       4, // "lib2.so" in string table
					},
				},
				Location: []*googlev1.Location{
					// Valid, pre-symbolized location
					{
						MappingId: 1,
						Address:   0x1234,
						Line: []*googlev1.Line{{
							FunctionId: 1,
							Line:       42,
						}},
					},
					// Location needing symbolization
					{
						MappingId: 2,
						Address:   0x1500, // This address exists in our test debug file
					},
				},
				Function: []*googlev1.Function{{
					Id:        1,
					Name:      5,
					Filename:  6,
					StartLine: 40,
				}},
				StringTable: []string{
					"",
					"already-symbolized-id",
					"lib1.so",
					"build-id",
					"lib2.so",
					"pre_symbolized_func",
					"pre_symbolized.c",
				},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				// The mapping flags should still be set
				require.True(t, p.Mapping[0].HasFunctions)
				require.True(t, p.Mapping[0].HasFilenames)
				require.True(t, p.Mapping[0].HasLineNumbers)

				// Second mapping should now be marked as symbolized
				require.True(t, p.Mapping[1].HasFunctions)
				require.True(t, p.Mapping[1].HasFilenames)
				require.True(t, p.Mapping[1].HasLineNumbers)

				// First location should be unchanged
				require.Len(t, p.Location[0].Line, 1)
				require.Equal(t, uint64(1), p.Location[0].Line[0].FunctionId)
				require.Equal(t, int64(42), p.Location[0].Line[0].Line)

				// Pre-symbolized function should still exist and be unchanged
				foundOriginalFunc := false
				for _, fn := range p.Function {
					if fn.Id == 1 {
						foundOriginalFunc = true
						require.Equal(t, int64(5), fn.Name)
						require.Equal(t, int64(6), fn.Filename)
						require.Equal(t, int64(40), fn.StartLine)
					}
				}
				require.True(t, foundOriginalFunc, "Original function should be preserved")

				// Second location should now be symbolized
				require.NotEmpty(t, p.Location[1].Line)

				// The second location should have correct symbols from the debug file
				assertLocationHasFunction(t, p, p.Location[1], "main",
					"/usr/src/stress-1.0.7-1/src/stress.c", 87)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient)

			s, err := NewProfileSymbolizer(log.NewNopLogger(), mockClient, NewNullDebugInfoStore(), NewMetrics(nil), 1, 1)
			require.NoError(t, err)

			err = s.SymbolizePprof(context.Background(), tt.profile)
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

func TestSymbolizeWithCache(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)

	openNewTestFile := func() io.ReadCloser {
		f, err := os.Open("testdata/symbols.debug")
		require.NoError(t, err)
		return f
	}

	// We expect exactly two calls to FetchDebuginfo:
	// 1. For the first request (build-id)
	// 2. For the fourth request (different-build-id)
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openNewTestFile(), nil).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "different-build-id").Return(openNewTestFile(), nil).Once()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Create a symbolizer with LRU cache
	s, err := NewProfileSymbolizer(nil, mockClient, NewNullDebugInfoStore(), metrics, 100000, 100000)
	require.NoError(t, err)

	// Request 1: First request - should be a cache miss for both symbol and debug info
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
	err = s.Symbolize(context.Background(), req1)
	require.NoError(t, err)
	require.NotEmpty(t, req1.Locations[0].Lines)

	// Wait for Ristretto to finish processing
	s.debugInfoCache.Wait()

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

	// Request 3: Same build-id, different address - should be a debug info cache hit
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

	s.debugInfoCache.Wait()

	// Should NOT call FetchDebuginfo again for the new address
	// because the debug info is already in the Ristretto cache

	err = s.Symbolize(context.Background(), req3)
	require.NoError(t, err)
	require.NotEmpty(t, req3.Locations[0].Lines)

	// Fourth request with a different build ID - should be a complete cache miss
	req4 := &Request{
		BuildID: "different-build-id",
		Locations: []*Location{
			{
				Address: 0x1500,
				Mapping: &pprof.Mapping{
					Start:   0x0,
					Limit:   0x1000000,
					Offset:  0x0,
					BuildID: "different-build-id",
				},
			},
		},
	}

	err = s.Symbolize(context.Background(), req4)
	require.NoError(t, err)
	require.NotEmpty(t, req4.Locations[0].Lines)

	mockClient.AssertExpectations(t)
}

func TestSymbolizerMetrics(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mocksymbolizer.MockDebuginfodClient)
		setupTest func(*ProfileSymbolizer, context.Context) error
		expected  map[string]int
	}{
		{
			name: "successful symbolization with cache layers",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			setupTest: func(s *ProfileSymbolizer, ctx context.Context) error {
				// First request - should miss all caches
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
				err := s.Symbolize(ctx, req1)
				if err != nil {
					return err
				}

				// Second request with same address - should hit symbol cache
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
				return s.Symbolize(ctx, req2)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":         1,
				"pyroscope_debug_symbol_resolution_duration_seconds":       2,
				"pyroscope_symbolizer_debuginfod_request_duration_seconds": 1,
				"pyroscope_symbolizer_cache_operation_duration_seconds":    6,
			},
		},
		{
			name: "debuginfod error",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "unknown-build-id").
					Return(nil, fmt.Errorf("unknown build ID")).Once()
			},
			setupTest: func(s *ProfileSymbolizer, ctx context.Context) error {
				req := &Request{
					BuildID: "unknown-build-id",
					Locations: []*Location{
						{
							Address: 0x1500,
							Mapping: &pprof.Mapping{
								Start:   0x0,
								Limit:   0x1000000,
								Offset:  0x0,
								BuildID: "unknown-build-id",
							},
						},
					},
				}
				_ = s.Symbolize(ctx, req)
				return nil
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":         1,
				"pyroscope_debug_symbol_resolution_duration_seconds":       1,
				"pyroscope_symbolizer_debuginfod_request_duration_seconds": 1,
				"pyroscope_symbolizer_cache_operation_duration_seconds":    2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			metrics := NewMetrics(reg)

			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient)

			s, err := NewProfileSymbolizer(nil, mockClient, NewNullDebugInfoStore(), metrics, 100, 100)
			require.NoError(t, err)

			err = tt.setupTest(s, context.Background())
			if err != nil {
				t.Log("Setup error:", err)
			}

			// Use testutil.GatherAndCount to check metric counts
			for metricName, expectedCount := range tt.expected {
				count, err := testutil.GatherAndCount(reg, metricName)
				require.NoError(t, err, "Error gathering metric %s", metricName)
				require.Equal(t, expectedCount, count, "Metric %s count mismatch", metricName)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func assertLocationHasFunction(t *testing.T, profile *googlev1.Profile, loc *googlev1.Location,
	functionName, fileName string, expectedLine int64) *googlev1.Function {
	t.Helper()

	found := false
	var targetFunction *googlev1.Function
	var actualLine int64

	for _, line := range loc.Line {
		// Find the function with this ID in the function table
		for _, fn := range profile.Function {
			if fn.Id == line.FunctionId {
				name := "<invalid>"
				if fn.Name >= 0 && int(fn.Name) < len(profile.StringTable) {
					name = profile.StringTable[fn.Name]
				}
				if name == functionName {
					found = true
					targetFunction = fn
					actualLine = line.Line
				}
			}
		}
	}

	require.True(t, found, "Function %q not found in location", functionName)

	if found {
		require.True(t, targetFunction.Filename >= 0 && int(targetFunction.Filename) < len(profile.StringTable),
			"Invalid filename index for function %q", functionName)
		require.Equal(t, fileName, profile.StringTable[targetFunction.Filename],
			"Wrong filename for function %q", functionName)
		require.Equal(t, expectedLine, actualLine,
			"Incorrect line number for function %q", functionName)
	}

	return targetFunction
}

// Helper function to open the test file
func openTestFile(t *testing.T) io.ReadCloser {
	t.Helper()
	f, err := os.Open("testdata/symbols.debug")
	require.NoError(t, err)
	return f
}
