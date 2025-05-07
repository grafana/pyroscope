package symbolizer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mocksymbolizer"

	"github.com/go-kit/log"
	pprof "github.com/google/pprof/profile"
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

			s := &Symbolizer{
				logger:  log.NewNopLogger(),
				client:  mockClient,
				store:   NewNullDebugInfoStore(),
				metrics: newMetrics(nil),
			}

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

func TestSymbolizationWithLidiaData(t *testing.T) {
	const testLidiaZip = "testdata/test_lidia_file.gz"
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"

	lidiaData, err := extractGzipFile(t, testLidiaZip)
	require.NoError(t, err)
	require.NotEmpty(t, lidiaData)

	store := mocksymbolizer.NewMockDebugInfoStore(t)

	// Configure the mock to return the same Lidia data for both Get operations
	getLidiaData := func() io.ReadCloser {
		return io.NopCloser(bytes.NewReader(lidiaData))
	}

	store.On("Get", mock.Anything, buildID).Return(getLidiaData(), nil).Once()
	store.On("Get", mock.Anything, buildID).Return(getLidiaData(), nil).Once()

	sym := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  nil,
		store:   store,
		metrics: newMetrics(prometheus.NewRegistry()),
	}

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
	require.NotEmpty(t, req.Locations[0].Lines)

	// Second request should also fetch from store
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
	require.NotEmpty(t, req2.Locations[0].Lines)
}

// TestSymbolizeWithObjectStore validates the symbolizer's behavior with the object store:
// 1. First request: Object store miss → fetch from debuginfod → store Lidia data in object store
// 2. Second request (same build-id, same address): Object store hit → use cached Lidia data
// 3. Third request (same build-id, different address): Object store hit → use cached Lidia data
// 4. Fourth request (different build-id): Object store miss → fetch from debuginfod → store Lidia data
func TestSymbolizeWithObjectStore(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
	mockStore := mocksymbolizer.NewMockDebugInfoStore(t)

	s := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  mockClient,
		store:   mockStore,
		metrics: newMetrics(prometheus.NewRegistry()),
	}

	elfTestFile := openTestFile(t)
	elfData, err := io.ReadAll(elfTestFile)
	elfTestFile.Close()
	require.NoError(t, err)

	// request 1
	var capturedLidiaData []byte
	mockStore.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(io.NopCloser(bytes.NewReader(elfData)), nil).Once()
	mockStore.On("Put", mock.Anything, "build-id", mock.Anything).Run(func(args mock.Arguments) {
		reader := args.Get(2).(io.Reader)
		var buf bytes.Buffer
		teeReader := io.TeeReader(reader, &buf)
		var err error
		capturedLidiaData, err = io.ReadAll(teeReader)
		require.NoError(t, err)
	}).Return(nil).Once()

	req1 := createRequest(t, "build-id", 0x1500)
	err = s.Symbolize(context.Background(), req1)
	require.NoError(t, err)
	require.NotEmpty(t, req1.Locations[0].Lines)
	require.NotEmpty(t, capturedLidiaData)

	// request 2
	mockStore.On("Get", mock.Anything, "build-id").Return(
		io.NopCloser(bytes.NewReader(capturedLidiaData)), nil,
	).Once()

	req2 := createRequest(t, "build-id", 0x1500)
	err = s.Symbolize(context.Background(), req2)
	require.NoError(t, err)
	require.NotEmpty(t, req2.Locations[0].Lines)

	// request 3
	mockStore.On("Get", mock.Anything, "build-id").Return(
		io.NopCloser(bytes.NewReader(capturedLidiaData)), nil,
	).Once()

	req3 := createRequest(t, "build-id", 0x3c5a)
	err = s.Symbolize(context.Background(), req3)
	require.NoError(t, err)
	require.NotEmpty(t, req3.Locations[0].Lines)

	// request 4
	var capturedLidiaData2 []byte
	mockStore.On("Get", mock.Anything, "different-build-id").Return(nil, fmt.Errorf("not found")).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "different-build-id").Return(io.NopCloser(bytes.NewReader(elfData)), nil).Once()
	mockStore.On("Put", mock.Anything, "different-build-id", mock.Anything).Run(func(args mock.Arguments) {
		reader := args.Get(2).(io.Reader)
		var buf bytes.Buffer
		teeReader := io.TeeReader(reader, &buf)
		var err error
		capturedLidiaData2, err = io.ReadAll(teeReader)
		require.NoError(t, err)
	}).Return(nil).Once()

	req4 := createRequest(t, "different-build-id", 0x1500)
	err = s.Symbolize(context.Background(), req4)
	require.NoError(t, err)
	require.NotEmpty(t, req4.Locations[0].Lines)
	require.NotEmpty(t, capturedLidiaData2)

	mockClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

func TestSymbolizerMetrics(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mocksymbolizer.MockDebuginfodClient, *mocksymbolizer.MockDebugInfoStore)
		setupTest func(*Symbolizer, context.Context) error
		expected  map[string]int
	}{
		{
			name: "successful symbolization with cache",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockStore *mocksymbolizer.MockDebugInfoStore) {
				// Get test data for proper Lidia format
				elfTestFile := openTestFile(t)
				elfData, err := io.ReadAll(elfTestFile)
				elfTestFile.Close()
				require.NoError(t, err)

				// Process the ELF data to get valid Lidia data for the cache hit
				preProcessor := &Symbolizer{
					logger:  log.NewNopLogger(),
					metrics: newMetrics(nil),
				}
				lidiaData, err := preProcessor.processELFData(elfData)
				require.NoError(t, err)
				require.NotEmpty(t, lidiaData)

				// First request: Object store miss, fetch from debuginfod
				mockStore.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(
					io.NopCloser(bytes.NewReader(elfData)), nil,
				).Once()
				mockStore.On("Put", mock.Anything, "build-id", mock.Anything).Return(nil).Once()

				// Second request: Object store hit using valid Lidia data
				mockStore.On("Get", mock.Anything, "build-id").Return(
					io.NopCloser(bytes.NewReader(lidiaData)), nil,
				).Once()
			},
			setupTest: func(s *Symbolizer, ctx context.Context) error {
				req1 := createRequest(t, "build-id", 0x1500)
				err := s.Symbolize(ctx, req1)
				if err != nil {
					return err
				}

				req2 := createRequest(t, "build-id", 0x1500)
				return s.Symbolize(ctx, req2)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":         1,
				"pyroscope_debug_symbol_resolution_duration_seconds":       1,
				"pyroscope_symbolizer_debuginfod_request_duration_seconds": 1,
			},
		},
		{
			name: "debuginfod error",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockStore *mocksymbolizer.MockDebugInfoStore) {
				mockStore.On("Get", mock.Anything, "unknown-build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockClient.On("FetchDebuginfo", mock.Anything, "unknown-build-id").
					Return(nil, buildIDNotFoundError{buildID: "unknown-build-id"}).Once()
			},
			setupTest: func(s *Symbolizer, ctx context.Context) error {
				req := createRequest(t, "unknown-build-id", 0x1500)
				return s.Symbolize(ctx, req)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":         1,
				"pyroscope_debug_symbol_resolution_duration_seconds":       0,
				"pyroscope_symbolizer_debuginfod_request_duration_seconds": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			mockStore := mocksymbolizer.NewMockDebugInfoStore(t)
			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient, mockStore)

			s := &Symbolizer{
				logger:  log.NewNopLogger(),
				client:  mockClient,
				store:   mockStore,
				metrics: newMetrics(reg),
			}

			err := tt.setupTest(s, context.Background())
			if err != nil {
				t.Log("Setup error:", err)
			}

			for metricName, expectedCount := range tt.expected {
				count, err := testutil.GatherAndCount(reg, metricName)
				require.NoError(t, err, "Error gathering metric %s", metricName)
				require.Equal(t, expectedCount, count, "Metric %s count mismatch", metricName)
			}

			mockClient.AssertExpectations(t)
			mockStore.AssertExpectations(t)
		})
	}
}

func assertLocationHasFunction(t *testing.T, profile *googlev1.Profile, loc *googlev1.Location,
	functionName, fileName string) {
	t.Helper()

	found := false

	for _, line := range loc.Line {
		for _, fn := range profile.Function {
			if fn.Id == line.FunctionId {
				name := "<invalid>"
				if fn.Name >= 0 && int(fn.Name) < len(profile.StringTable) {
					name = profile.StringTable[fn.Name]
				}
				if name == functionName {
					found = true
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
	}

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

func createRequest(t *testing.T, buildID string, address uint64) *Request {
	t.Helper()
	return &Request{
		BuildID: buildID,
		Locations: []*Location{
			{
				Address: address,
				Mapping: &pprof.Mapping{
					Start:   0x0,
					Limit:   0x1000000,
					Offset:  0x0,
					BuildID: buildID,
				},
			},
		},
	}
}
