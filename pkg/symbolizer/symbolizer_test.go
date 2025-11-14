package symbolizer

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockobjstore"
	"github.com/grafana/pyroscope/pkg/test/mocks/mocksymbolizer"

	"github.com/go-kit/log"
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
		setupMock func(*mocksymbolizer.MockDebuginfodClient, *mockobjstore.MockBucket)
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
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {},
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
					Id:        1,
					MappingId: 1,
					Address:   0x1500,
				}},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
				mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Return(nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)

				require.Len(t, p.Location[0].Line, 1)

				assertLocationHasFunction(t, p, p.Location[0], "main", "main")
			},
		},
		{
			name: "empty build ID creates fallback symbols",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Id:       1,
					Filename: 2,
					BuildId:  1,
				}},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xa4c},
					{Id: 2, MappingId: 1, Address: 0x9f0},
				},
				StringTable: []string{"", "", "linux-vdso.1.so"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)
				require.Len(t, p.Location[0].Line, 1)
				require.Len(t, p.Location[1].Line, 1)

				fn1 := p.StringTable[p.Function[p.Location[0].Line[0].FunctionId-1].Name]
				fn2 := p.StringTable[p.Function[p.Location[1].Line[0].FunctionId-1].Name]
				require.Contains(t, fn1, "linux-vdso.1.so")
				require.Contains(t, fn1, "0xa4c")
				require.Contains(t, fn2, "linux-vdso.1.so")
				require.Contains(t, fn2, "0x9f0")
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
					{Id: 1, MappingId: 1, Address: 0x1500},
					{Id: 2, MappingId: 1, Address: 0x3b60},
					{Id: 3, MappingId: 1, Address: 0x1440},
				},
				StringTable: []string{"", "build-id"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
				mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Return(nil).Once()
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
		{
			name: "preserve existing symbols when HasFunctions=false",
			// This tests a defensive check against data inconsistency where a mapping has
			// HasFunctions=false but contains locations with existing symbols.
			// This scenario should be rare, but we maintain the check for robustness.
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Id:           1,
					BuildId:      1,
					Filename:     2,
					MemoryStart:  0x0,
					MemoryLimit:  0x1000000,
					FileOffset:   0x0,
					HasFunctions: false,
				}},
				Location: []*googlev1.Location{
					{
						Id:        1,
						MappingId: 1,
						Address:   0x1000,
						Line: []*googlev1.Line{{
							FunctionId: 1,
							Line:       42,
						}},
					},
					{
						Id:        2,
						MappingId: 1,
						Address:   0x1500,
						Line:      nil,
					},
				},
				Function: []*googlev1.Function{{
					Id:   1,
					Name: 3,
				}},
				StringTable: []string{"", "build-id", "alloy", "existing_function"},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()
				mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Return(nil).Once()
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)

				require.Len(t, p.Location[0].Line, 1)
				require.Equal(t, uint64(1), p.Location[0].Line[0].FunctionId)
				require.Equal(t, "existing_function", p.StringTable[p.Function[0].Name])

				require.Len(t, p.Location[1].Line, 1)
				assertLocationHasFunction(t, p, p.Location[1], "main", "main")

				existingFuncStillExists := false
				for _, str := range p.StringTable {
					if str == "existing_function" {
						existingFuncStillExists = true
						break
					}
				}
				require.True(t, existingFuncStillExists)

				placeholderFound := false
				for _, str := range p.StringTable {
					if strings.Contains(str, "!0x") {
						placeholderFound = true
						break
					}
				}
				require.False(t, placeholderFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			mockBucket := mockobjstore.NewMockBucket(t)
			tt.setupMock(mockClient, mockBucket)

			s := &Symbolizer{
				logger:  log.NewNopLogger(),
				client:  mockClient,
				bucket:  mockBucket,
				metrics: newMetrics(nil),
				cfg:     Config{MaxDebuginfodConcurrency: 1},
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

func TestSymbolizationKeepsSequentialFunctionIDs(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
	mockBucket := mockobjstore.NewMockBucket(t)

	profile := &googlev1.Profile{
		Mapping:     []*googlev1.Mapping{{BuildId: 1}},
		Location:    []*googlev1.Location{{Id: 1, MappingId: 1, Address: 0x1500}},
		Function:    []*googlev1.Function{{Id: 1, Name: 1}},
		StringTable: []string{"", "build-id", "existing_func"},
		Sample: []*googlev1.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{100},
		}},
	}

	mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found"))
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil)
	mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Return(nil)

	s := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  mockClient,
		bucket:  mockBucket,
		metrics: newMetrics(nil),
		cfg:     Config{MaxDebuginfodConcurrency: 1},
	}

	err := s.SymbolizePprof(context.Background(), profile)
	require.NoError(t, err)

	// Verify sequential function IDs
	for i, fn := range profile.Function {
		require.Equal(t, uint64(i+1), fn.Id)
	}

	_, err = model.TreeFromBackendProfile(profile, 1000)
	require.NoError(t, err)
}

func TestSymbolizationWithLidiaData(t *testing.T) {
	const testLidiaZip = "testdata/test_lidia_file.gz"
	const buildID = "ffcf60c240417166980a43fbbfde486e0b3718e5"

	lidiaData, err := extractGzipFile(t, testLidiaZip)
	require.NoError(t, err)
	require.NotEmpty(t, lidiaData)

	// Configure the mock to return the same Lidia data for both Get operations
	getLidiaData := func() io.ReadCloser {
		return io.NopCloser(bytes.NewReader(lidiaData))
	}

	mockBucket := mockobjstore.NewMockBucket(t)
	mockBucket.On("Get", mock.Anything, buildID).Return(getLidiaData(), nil).Once()
	mockBucket.On("Get", mock.Anything, buildID).Return(getLidiaData(), nil).Once()

	sym := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  nil,
		bucket:  mockBucket,
		metrics: newMetrics(prometheus.NewRegistry()),
		cfg:     Config{MaxDebuginfodConcurrency: 1},
	}

	req := &request{
		buildID:    buildID,
		binaryName: "test-binary",
		locations: []*location{
			{
				address: 0x1b743d6,
			},
		},
	}

	sym.symbolize(context.Background(), req)
	require.NotEmpty(t, req.locations[0].lines)

	// Second request should also fetch from store
	req2 := &request{
		buildID:    buildID,
		binaryName: "test-binary",
		locations: []*location{
			{
				address: 0x1b743d6,
			},
		},
	}

	sym.symbolize(context.Background(), req2)
	require.NotEmpty(t, req2.locations[0].lines)
}

// TestSymbolizeWithObjectStore validates the symbolizer's behavior with the object store:
// 1. First request: Object store miss → fetch from debuginfod → store Lidia data in object store
// 2. Second request (same build-id, same address): Object store hit → use cached Lidia data
// 3. Third request (same build-id, different address): Object store hit → use cached Lidia data
// 4. Fourth request (different build-id): Object store miss → fetch from debuginfod → store Lidia data
func TestSymbolizeWithObjectStore(t *testing.T) {
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
	mockBucket := mockobjstore.NewMockBucket(t)

	s := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  mockClient,
		bucket:  mockBucket,
		metrics: newMetrics(prometheus.NewRegistry()),
		cfg:     Config{MaxDebuginfodConcurrency: 1},
	}

	elfTestFile := openTestFile(t)
	elfData, err := io.ReadAll(elfTestFile)
	elfTestFile.Close()
	require.NoError(t, err)

	// request 1
	var capturedLidiaData []byte
	mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(io.NopCloser(bytes.NewReader(elfData)), nil).Once()
	mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Run(func(args mock.Arguments) {
		reader := args.Get(2).(io.Reader)
		var buf bytes.Buffer
		teeReader := io.TeeReader(reader, &buf)
		var err error
		capturedLidiaData, err = io.ReadAll(teeReader)
		require.NoError(t, err)
	}).Return(nil).Once()

	req1 := createRequest(t, "build-id", 0x1500)
	s.symbolize(context.Background(), req1)
	require.NotEmpty(t, req1.locations[0].lines)
	require.NotEmpty(t, capturedLidiaData)

	// request 2
	mockBucket.On("Get", mock.Anything, "build-id").Return(
		io.NopCloser(bytes.NewReader(capturedLidiaData)), nil,
	).Once()

	req2 := createRequest(t, "build-id", 0x1500)
	s.symbolize(context.Background(), req2)
	require.NotEmpty(t, req2.locations[0].lines)

	// request 3
	mockBucket.On("Get", mock.Anything, "build-id").Return(
		io.NopCloser(bytes.NewReader(capturedLidiaData)), nil,
	).Once()

	req3 := createRequest(t, "build-id", 0x3c5a)
	s.symbolize(context.Background(), req3)
	require.NotEmpty(t, req3.locations[0].lines)

	// request 4
	var capturedLidiaData2 []byte
	mockBucket.On("Get", mock.Anything, "different-build-id").Return(nil, fmt.Errorf("not found")).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "different-build-id").Return(io.NopCloser(bytes.NewReader(elfData)), nil).Once()
	mockBucket.On("Upload", mock.Anything, "different-build-id", mock.Anything).Run(func(args mock.Arguments) {
		reader := args.Get(2).(io.Reader)
		var buf bytes.Buffer
		teeReader := io.TeeReader(reader, &buf)
		var err error
		capturedLidiaData2, err = io.ReadAll(teeReader)
		require.NoError(t, err)
	}).Return(nil).Once()

	req4 := createRequest(t, "different-build-id", 0x1500)
	s.symbolize(context.Background(), req4)
	require.NotEmpty(t, req4.locations[0].lines)
	require.NotEmpty(t, capturedLidiaData2)

	mockClient.AssertExpectations(t)
	mockBucket.AssertExpectations(t)
}

func TestSymbolizerMetrics(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mocksymbolizer.MockDebuginfodClient, *mockobjstore.MockBucket)
		setupTest func(*Symbolizer, context.Context)
		expected  map[string]int
	}{
		{
			name: "successful symbolization with cache",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				elfTestFile := openTestFile(t)
				elfData, err := io.ReadAll(elfTestFile)
				elfTestFile.Close()
				require.NoError(t, err)

				preProcessor := &Symbolizer{
					logger:  log.NewNopLogger(),
					metrics: newMetrics(nil),
				}
				lidiaData, err := preProcessor.processELFData(elfData)
				require.NoError(t, err)
				require.NotEmpty(t, lidiaData)

				mockBucket.On("IsObjNotFoundErr", mock.Anything).Return(true).Maybe()
				mockBucket.On("Name").Return("test-bucket").Maybe()

				mockBucket.On("Get", mock.Anything, "build-id").Return(nil, fmt.Errorf("not found")).Once()

				mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(
					io.NopCloser(bytes.NewReader(elfData)), nil,
				).Once()
				mockBucket.On("Upload", mock.Anything, "build-id", mock.Anything).Return(nil).Once()

				mockBucket.On("Get", mock.Anything, "build-id").Return(
					io.NopCloser(bytes.NewReader(lidiaData)), nil,
				).Once()
			},
			setupTest: func(s *Symbolizer, ctx context.Context) {
				req1 := createRequest(t, "build-id", 0x1500)
				s.symbolize(ctx, req1)

				req2 := createRequest(t, "build-id", 0x1500)
				s.symbolize(ctx, req2)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":   0,
				"pyroscope_debug_symbol_resolution_duration_seconds": 1,
				"pyroscope_debug_symbol_resolution_errors_total":     0,
			},
		},
		{
			name: "debuginfod error",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				mockBucket.On("Get", mock.Anything, "unknown-build-id").Return(nil, fmt.Errorf("not found")).Once()
				mockClient.On("FetchDebuginfo", mock.Anything, "unknown-build-id").
					Return(nil, buildIDNotFoundError{buildID: "unknown-build-id"}).Once()
			},
			setupTest: func(s *Symbolizer, ctx context.Context) {
				req := createRequest(t, "unknown-build-id", 0x1500)
				s.symbolize(ctx, req)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds":   0,
				"pyroscope_debug_symbol_resolution_duration_seconds": 0,
				"pyroscope_debug_symbol_resolution_errors_total":     0,
			},
		},
		{
			name: "elf_parsing_error",
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {
				invalidData := []byte("invalid elf data")

				mockBucket.On("Get", mock.Anything, "invalid-elf").Return(nil, fmt.Errorf("not found")).Once()
				mockClient.On("FetchDebuginfo", mock.Anything, "invalid-elf").Return(
					io.NopCloser(bytes.NewReader(invalidData)), nil,
				).Once()
			},
			setupTest: func(s *Symbolizer, ctx context.Context) {
				req := createRequest(t, "invalid-elf", 0x1500)
				s.symbolize(ctx, req)
			},
			expected: map[string]int{
				"pyroscope_profile_symbolization_duration_seconds": 0,
				"pyroscope_debug_symbol_resolution_errors_total":   1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			mockBucket := mockobjstore.NewMockBucket(t)
			mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
			tt.setupMock(mockClient, mockBucket)

			s := &Symbolizer{
				logger:  log.NewNopLogger(),
				client:  mockClient,
				bucket:  mockBucket,
				metrics: newMetrics(reg),
			}

			tt.setupTest(s, context.Background())

			for metricName, expectedCount := range tt.expected {
				count, err := testutil.GatherAndCount(reg, metricName)
				require.NoError(t, err, "Error gathering metric %s", metricName)
				require.Equal(t, expectedCount, count, "Metric %s count mismatch", metricName)
			}

			mockClient.AssertExpectations(t)
			mockBucket.AssertExpectations(t)
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

func createRequest(t *testing.T, buildID string, address uint64) *request {
	t.Helper()
	return &request{
		buildID: buildID,
		locations: []*location{
			{
				address: address,
			},
		},
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(cfg *Config)
		wantErr bool
	}{
		{
			name:    "valid config with positive concurrency",
			setup:   func(cfg *Config) { cfg.MaxDebuginfodConcurrency = 10 },
			wantErr: false,
		},
		{
			name:    "invalid config with zero concurrency",
			setup:   func(cfg *Config) { cfg.MaxDebuginfodConcurrency = 0 },
			wantErr: true,
		},
		{
			name:    "invalid config with negative concurrency",
			setup:   func(cfg *Config) { cfg.MaxDebuginfodConcurrency = -1 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			tt.setup(&cfg)
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpdateAllSymbolsInProfile verifies that line numbers, file paths, and StartLine
// are properly passed through from SourceInfoFrame to the profile.
func TestUpdateAllSymbolsInProfile(t *testing.T) {
	s := &Symbolizer{logger: log.NewNopLogger()}
	stringMap := make(map[string]int64)

	t.Run("basic symbolization", func(t *testing.T) {
		profile := &googlev1.Profile{
			Mapping:     []*googlev1.Mapping{{Id: 1, HasFunctions: false}},
			Location:    []*googlev1.Location{{Id: 1, MappingId: 1, Address: 0x1500}},
			StringTable: []string{""},
			Function:    []*googlev1.Function{},
		}

		symbolizedLocs := []symbolizedLocation{{
			loc: profile.Location[0],
			symLoc: &location{
				address: 0x1500,
				lines: []lidia.SourceInfoFrame{{
					LineNumber: 42, FunctionName: "testFunction", FilePath: "/path/to/test.go",
				}},
			},
			mapping: profile.Mapping[0],
		}}

		s.updateAllSymbolsInProfile(profile, symbolizedLocs, stringMap)

		require.True(t, profile.Mapping[0].HasFunctions)
		require.Len(t, profile.Location[0].Line, 1)
		require.Len(t, profile.Function, 1)

		line := profile.Location[0].Line[0]
		fn := profile.Function[0]

		require.Equal(t, int64(42), line.Line)
		require.Equal(t, int64(42), fn.StartLine)
		require.Equal(t, "testFunction", profile.StringTable[fn.Name])
		require.Equal(t, "/path/to/test.go", profile.StringTable[fn.Filename])
	})

	t.Run("minimum StartLine for same function", func(t *testing.T) {
		profile := &googlev1.Profile{
			Mapping: []*googlev1.Mapping{{Id: 1, HasFunctions: false}},
			Location: []*googlev1.Location{
				{Id: 1, MappingId: 1, Address: 0x1500},
				{Id: 2, MappingId: 1, Address: 0x1600},
			},
			StringTable: []string{""},
			Function:    []*googlev1.Function{},
		}

		symbolizedLocs := []symbolizedLocation{
			{
				loc: profile.Location[0],
				symLoc: &location{address: 0x1500, lines: []lidia.SourceInfoFrame{{
					LineNumber: 100, FunctionName: "testFunction", FilePath: "/path/to/test.go",
				}}},
				mapping: profile.Mapping[0],
			},
			{
				loc: profile.Location[1],
				symLoc: &location{address: 0x1600, lines: []lidia.SourceInfoFrame{{
					LineNumber: 50, FunctionName: "testFunction", FilePath: "/path/to/test.go",
				}}},
				mapping: profile.Mapping[0],
			},
		}

		s.updateAllSymbolsInProfile(profile, symbolizedLocs, stringMap)

		require.Len(t, profile.Function, 1)
		// StartLine properly updated
		require.Equal(t, int64(50), profile.Function[0].StartLine)
		require.Equal(t, int64(100), profile.Location[0].Line[0].Line)
		require.Equal(t, int64(50), profile.Location[1].Line[0].Line)
	})
}
