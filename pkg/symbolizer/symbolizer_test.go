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
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
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
			name: "empty build ID",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId: 1,
				}},
				StringTable: []string{"", ""},
			},
			setupMock: func(mockClient *mocksymbolizer.MockDebuginfodClient, mockBucket *mockobjstore.MockBucket) {},
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
	s := createSymbolizerForBuildID(t, "build-id")

	profile := &googlev1.Profile{
		Mapping:     []*googlev1.Mapping{{BuildId: 1}},
		Location:    []*googlev1.Location{{Id: 1, MappingId: 1, Address: 0x1500}},
		Function:    []*googlev1.Function{{Id: 1, Name: 2}},
		StringTable: []string{"", "build-id", "existing_func"},
		Sample: []*googlev1.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{100},
		}},
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

// TestMixedSymbolizationCorrectsFlagsForAllMappings tests that the symbolizer correctly sets HasFunctions=false for
// mappings with partial symbolization, fixing incorrect flags from upstream sources.
func TestMixedSymbolizationCorrectsFlagsForAllMappings(t *testing.T) {
	// Create a profile with incorrect HasFunctions=true on a mixed mapping
	profile := &googlev1.Profile{
		Mapping: []*googlev1.Mapping{
			{Id: 1, HasFunctions: true, BuildId: 1, Filename: 1}, // incorrect: has mixed symbolization
		},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Address: 0x1000, Line: []*googlev1.Line{{FunctionId: 1}}}, // symbolized
			{Id: 2, MappingId: 1, Address: 0x2000, Line: nil},                               // unsymbolized
		},
		Function:    []*googlev1.Function{{Id: 1, Name: 2}},
		StringTable: []string{"", "test.so", "existing_func"},
	}

	profile.StringTable[profile.Mapping[0].BuildId] = "" // Make buildID empty so symbolizer skips it

	s := &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  nil,
		bucket:  nil,
		metrics: newMetrics(nil),
	}

	err := s.SymbolizePprof(context.Background(), profile)
	require.NoError(t, err)

	// Verify that HasFunctions was corrected to false for mixed symbolization
	require.False(t, profile.Mapping[0].HasFunctions, "Mapping with mixed symbolization should have HasFunctions=false")
}

// createSymbolizerForBuildID creates a symbolizer with mocks for a specific buildID
func createSymbolizerForBuildID(t *testing.T, buildID string) *Symbolizer {
	t.Helper()
	mockClient := mocksymbolizer.NewMockDebuginfodClient(t)
	mockBucket := mockobjstore.NewMockBucket(t)

	mockBucket.On("Get", mock.Anything, buildID).Return(nil, fmt.Errorf("not found"))
	mockClient.On("FetchDebuginfo", mock.Anything, buildID).Return(openTestFile(t), nil)
	mockBucket.On("Upload", mock.Anything, buildID, mock.Anything).Return(nil)

	return &Symbolizer{
		logger:  log.NewNopLogger(),
		client:  mockClient,
		bucket:  mockBucket,
		metrics: newMetrics(nil),
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

// TestFlamegraphTruncationIntegration is an integration test for the flamegraph truncation fix.
// TODO: Move this to an integration test package when one exists.
//
// Context: Mixed symbolization profiles with incorrect HasFunctions=true flags caused flamegraph truncation.
// When clearAddresses() cleared ALL addresses during normalize(), unsymbolized locations became
// indistinguishable (identical LocationKeys) and merged incorrectly, losing flamegraph blocks.
//
// Symbolizer now corrects HasFunctions flags based on actual symbolization state.
// Mixed mappings get HasFunctions=false, preserving addresses for proper deduplication.
func TestFlamegraphTruncationIntegration(t *testing.T) {
	symbolizer := &Symbolizer{
		logger:  log.NewNopLogger(),
		metrics: newMetrics(prometheus.NewRegistry()),
	}

	// Create profile with mixed symbolization - some locations already symbolized
	profile := &googlev1.Profile{
		SampleType: []*googlev1.ValueType{{Type: 1, Unit: 2}},
		PeriodType: &googlev1.ValueType{Type: 1, Unit: 2},
		Period:     1000000,
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{1, 2}, Value: []int64{100}},
			{LocationId: []uint64{3, 2}, Value: []int64{200}},
		},
		Mapping: []*googlev1.Mapping{{
			Id: 1, HasFunctions: true, BuildId: 3, Filename: 4, // Incorrectly set to true for mixed mapping
			MemoryStart: 0x400000, MemoryLimit: 0x500000,
		}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Address: 0x1500, Line: []*googlev1.Line{{FunctionId: 1}}}, // symbolized location
			{Id: 2, MappingId: 1, Address: 0x999999, Line: nil},
			{Id: 3, MappingId: 1, Address: 0x888888, Line: nil},
		},
		Function: []*googlev1.Function{
			{Id: 1, Name: 5}, // Function for the symbolized location
		},
		StringTable: []string{"", "samples", "count", "build-id", "test.so", "symbolized_function"},
	}

	// Symbolizer corrects HasFunctions flag for mixed mapping
	err := symbolizer.SymbolizePprof(context.Background(), profile)
	require.NoError(t, err)
	require.False(t, profile.Mapping[0].HasFunctions, "Mixed mapping should have HasFunctions=false")

	// Profile normalization preserves addresses when HasFunctions=false
	pprofProfile := &pprof.Profile{Profile: profile}
	pprofProfile.Normalize() // calls clearAddresses()

	require.NotZero(t, profile.Location[0].Address, "Address should be preserved when HasFunctions=false")
	require.NotZero(t, profile.Location[1].Address, "Address should be preserved when HasFunctions=false")
	require.NotZero(t, profile.Location[2].Address, "Address should be preserved when HasFunctions=false")

	// Profile merge preserves all distinct locations
	var merge pprof.ProfileMerge
	err = merge.Merge(profile, true)
	require.NoError(t, err)

	mergedProfile := merge.Profile()
	require.Equal(t, 3, len(mergedProfile.Location), "All locations should be preserved with distinct addresses")

	// Verify unsymbolized locations maintain their distinct addresses
	var unsymbolizedAddrs []uint64
	for _, loc := range mergedProfile.Location {
		if len(loc.Line) == 0 {
			unsymbolizedAddrs = append(unsymbolizedAddrs, loc.Address)
		}
	}
	require.Len(t, unsymbolizedAddrs, 2, "Both unsymbolized locations should be preserved")
	require.Contains(t, unsymbolizedAddrs, uint64(0x999999))
	require.Contains(t, unsymbolizedAddrs, uint64(0x888888))
}
