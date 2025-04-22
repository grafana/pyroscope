package symbolizer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/go-kit/log"
	pprof "github.com/google/pprof/profile"
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mocksymbolizer"
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
			name: "needs symbolization single address",
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

				require.Len(t, p.Location[0].Line, 1)

				assertLocationHasFunction(t, p, p.Location[0], "main", "main")
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
					{MappingId: 1, Address: 0x3b60},
					{MappingId: 1, Address: 0x1440},
				},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)

				// First location (0x1500) - main
				require.Len(t, p.Location[0].Line, 1)
				assertLocationHasFunction(t, p, p.Location[0], "main", "main")

				// Second location (0x3b60) - atoll_b
				require.Len(t, p.Location[1].Line, 1)
				assertLocationHasFunction(t, p, p.Location[1], "atoll_b", "atoll_b")

				// Third location (0x1440) - main
				require.Len(t, p.Location[2].Line, 1)
				assertLocationHasFunction(t, p, p.Location[2], "main", "main")
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

func TestSymbolizationWithLidiaData(t *testing.T) {
	const testLidiaZip = "testdata/test_lidia_file.gz"
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"

	lidiaData, err := extractGzipFile(t, testLidiaZip)
	require.NoError(t, err)
	require.NotEmpty(t, lidiaData, "Lidia data should not be empty")

	store := mocksymbolizer.NewMockDebugInfoStore(t)
	store.On("Put", mock.Anything, buildID, mock.Anything).Return(nil).Once()
	store.On("Get", mock.Anything, buildID).Return(io.NopCloser(bytes.NewReader(lidiaData)), nil).Once()

	// Store the Lidia file
	err = store.Put(context.Background(), buildID, bytes.NewReader(lidiaData))
	require.NoError(t, err)

	sym, err := NewProfileSymbolizer(log.NewNopLogger(), nil, store, NewMetrics(nil), 100, 1024*1024)
	require.NoError(t, err)

	// Create a symbolization request with some addresses to test
	req := &Request{
		BuildID:    buildID,
		BinaryName: "test-binary",
		Locations: []*Location{
			{
				Address: 0x1b743d6,
				Mapping: &pprof.Mapping{
					Start:   0x403000,
					Limit:   0x1d75000,
					Offset:  0x3000,
					BuildID: buildID,
				},
			},
		},
	}

	err = sym.Symbolize(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, req.Locations[0].Lines, "Should have found symbols")
	for _, line := range req.Locations[0].Lines {
		t.Logf("Found symbol: %s in %s:%d", line.FunctionName, line.FilePath, line.LineNumber)
	}

	// For the second request, we don't need to set up expectations again
	// because the symbolizer should use its internal cache
	req2 := &Request{
		BuildID:    buildID,
		BinaryName: "test-binary",
		Locations: []*Location{
			{
				Address: 0x1b743d6,
				Mapping: &pprof.Mapping{
					Start:   0x403000,
					Limit:   0x1d75000,
					Offset:  0x3000,
					BuildID: buildID,
				},
			},
		},
	}

	err = sym.Symbolize(context.Background(), req2)
	require.NoError(t, err)
	require.NotEmpty(t, req2.Locations[0].Lines, "Should have found symbols from cache")
}

func TestSymbolizeWithCache(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)

	openNewTestFile := func() io.ReadCloser {
		// Read the entire file into memory to avoid issues with file handles
		f, err := os.Open("testdata/symbols.debug")
		require.NoError(t, err)
		defer f.Close()

		data, err := io.ReadAll(f)
		require.NoError(t, err)

		// Return a reader that reads from the in-memory data
		return io.NopCloser(bytes.NewReader(data))
	}

	// We expect exactly two calls to FetchDebuginfo:
	// 1. For the first request (build-id)
	// 2. For the fourth request (different-build-id)
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openNewTestFile(), nil).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "different-build-id").Return(openNewTestFile(), nil).Once()

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

	// Create a symbolizer with LRU cache
	s, err := NewProfileSymbolizer(log.NewNopLogger(), mockClient, NewNullDebugInfoStore(), metrics, 100000, 100000)
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
	s.lidiaTableCache.Wait()

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

	s.lidiaTableCache.Wait()

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

			s, err := NewProfileSymbolizer(log.NewNopLogger(), mockClient, NewNullDebugInfoStore(), metrics, 100, 100)
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
	functionName, fileName string) *googlev1.Function {
	t.Helper()

	found := false
	var targetFunction *googlev1.Function

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
				}
			}
		}
	}

	require.True(t, found, "Function %q not found in location", functionName)

	if found {
		fileNameFound := false
		for _, str := range profile.StringTable {
			if str == fileName {
				fileNameFound = true
				break
			}
		}
		require.True(t, fileNameFound, "Filename %q not found in string table", fileName)
		// We don't check line numbers until supported
	}

	return targetFunction
}

func openTestFile(t *testing.T) io.ReadCloser {
	t.Helper()
	f, err := os.Open("testdata/symbols.debug")
	require.NoError(t, err)

	data, err := io.ReadAll(f)
	require.NoError(t, err)
	f.Close()

	return NewReaderAtCloser(data)
}

func extractGzipFile(t *testing.T, gzipPath string) ([]byte, error) {
	t.Helper()
	file, err := os.Open(gzipPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	return io.ReadAll(gzipReader)
}

func extractGzipFile(t *testing.T, gzipPath string) ([]byte, error) {
	t.Helper()
	file, err := os.Open(gzipPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	return io.ReadAll(gzipReader)
}
