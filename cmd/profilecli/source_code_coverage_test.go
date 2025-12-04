package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestExtractFunctions(t *testing.T) {
	tests := []struct {
		name     string
		profile  *profilev1.Profile
		expected []config.FileSpec
	}{
		{
			name: "extract functions with names and paths",
			profile: &profilev1.Profile{
				StringTable: []string{"", "main", "foo", "bar", "/path/to/main.go", "/path/to/bar.go"},
				Function: []*profilev1.Function{
					{Id: 1, Name: 1, Filename: 4}, // main in /path/to/main.go
					{Id: 2, Name: 2, Filename: 4}, // foo in /path/to/main.go
					{Id: 3, Name: 3, Filename: 5}, // bar in /path/to/bar.go
				},
			},
			expected: []config.FileSpec{
				{FunctionName: "main", Path: "/path/to/main.go"},
				{FunctionName: "foo", Path: "/path/to/main.go"},
				{FunctionName: "bar", Path: "/path/to/bar.go"},
			},
		},
		{
			name: "skip functions with no name or path",
			profile: &profilev1.Profile{
				StringTable: []string{"", "main", "/path/to/main.go"},
				Function: []*profilev1.Function{
					{Id: 1, Name: 1, Filename: 2}, // main in /path/to/main.go
					{Id: 2, Name: 0, Filename: 0}, // no name or path - should be skipped
				},
			},
			expected: []config.FileSpec{
				{FunctionName: "main", Path: "/path/to/main.go"},
			},
		},
		{
			name: "deduplicate functions",
			profile: &profilev1.Profile{
				StringTable: []string{"", "main", "/path/to/main.go"},
				Function: []*profilev1.Function{
					{Id: 1, Name: 1, Filename: 2}, // main in /path/to/main.go
					{Id: 2, Name: 1, Filename: 2}, // duplicate - should be skipped
				},
			},
			expected: []config.FileSpec{
				{FunctionName: "main", Path: "/path/to/main.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFunctions(tt.profile)
			require.Equal(t, len(tt.expected), len(result))
			for i, expected := range tt.expected {
				require.Equal(t, expected.FunctionName, result[i].FunctionName)
				require.Equal(t, expected.Path, result[i].Path)
			}
		})
	}
}

func TestCalculateSampleCountsMap(t *testing.T) {
	profile := &profilev1.Profile{
		StringTable: []string{"", "main", "foo", "/path/to/main.go", "/path/to/foo.go"},
		Function: []*profilev1.Function{
			{Id: 1, Name: 1, Filename: 3}, // main in /path/to/main.go
			{Id: 2, Name: 2, Filename: 4}, // foo in /path/to/foo.go
		},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 10}}},
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 20}}},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{5}},    // 5 samples for main
			{LocationId: []uint64{1}, Value: []int64{3}},    // 3 more samples for main
			{LocationId: []uint64{2}, Value: []int64{2}},    // 2 samples for foo
			{LocationId: []uint64{1, 2}, Value: []int64{1}}, // 1 sample for both (should count both)
		},
	}

	result := calculateSampleCountsMap(profile)

	require.Equal(t, int64(9), result["main|/path/to/main.go"]) // 5 + 3 + 1
	require.Equal(t, int64(3), result["foo|/path/to/foo.go"])   // 2 + 1
}

func TestGenerateOutput(t *testing.T) {
	report := &coverageReport{
		TotalFunctions:     10,
		CoveredFunctions:   7,
		UncoveredFunctions: 3,
		CoveragePercentage: 70.0,
		Results: []functionResult{
			{FunctionName: "main", Path: "/main.go", Covered: true, SampleCount: 100},
			{FunctionName: "foo", Path: "/foo.go", Covered: false, SampleCount: 50},
		},
	}

	t.Run("text format", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := generateOutput(report, "text")
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		output := make([]byte, 1024)
		n, _ := r.Read(output)
		outputStr := string(output[:n])

		require.Contains(t, outputStr, "Coverage Summary")
		require.Contains(t, outputStr, "Total Functions:     10")
		require.Contains(t, outputStr, "Covered Functions:   7")
		require.Contains(t, outputStr, "Coverage:            70.00%")
	})

	t.Run("detailed format", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := generateOutput(report, "detailed")
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		output := make([]byte, 1024)
		n, _ := r.Read(output)
		outputStr := string(output[:n])

		require.Contains(t, outputStr, "Detailed Results")
		require.Contains(t, outputStr, "main")
		require.Contains(t, outputStr, "foo")
	})

	t.Run("unknown format", func(t *testing.T) {
		err := generateOutput(report, "unknown")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown output format")
	})
}

func TestListAllFunctions(t *testing.T) {
	// Create a temporary profile file
	builder := testhelper.NewProfileBuilder(1000).
		CPUProfile().
		ForStacktraceString("main", "foo", "bar").AddSamples(10)

	profileBytes, err := builder.MarshalVT()
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp("", "test-profile-*.pprof")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(profileBytes)
	require.NoError(t, err)
	tmpFile.Close()

	t.Run("text output", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := listAllFunctions(tmpFile.Name())
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		output := make([]byte, 1024)
		n, _ := r.Read(output)
		outputStr := string(output[:n])

		require.Contains(t, outputStr, "Functions in Profile")
		require.Contains(t, outputStr, "Total:")
	})

	t.Run("invalid profile file", func(t *testing.T) {
		err := listAllFunctions("/nonexistent/file.pprof")
		require.Error(t, err)
	})
}

func TestCoverageReportSortBySampleCount(t *testing.T) {
	report := &coverageReport{
		Results: []functionResult{
			{FunctionName: "low", SampleCount: 10},
			{FunctionName: "high", SampleCount: 100},
			{FunctionName: "medium", SampleCount: 50},
		},
	}

	report.sortBySampleCount()

	require.Equal(t, "high", report.Results[0].FunctionName)
	require.Equal(t, int64(100), report.Results[0].SampleCount)
	require.Equal(t, "medium", report.Results[1].FunctionName)
	require.Equal(t, int64(50), report.Results[1].SampleCount)
	require.Equal(t, "low", report.Results[2].FunctionName)
	require.Equal(t, int64(10), report.Results[2].SampleCount)
}

func TestHybridVCSClient(t *testing.T) {
	configContent := []byte("source_code:\n  mappings: []")
	configPath := ".pyroscope.yaml"

	mockClient := &mockVCSClient{}
	hybridClient := &hybridVCSClient{
		configContent: configContent,
		configPath:    configPath,
		realClient:    mockClient,
	}

	t.Run("intercepts config file requests", func(t *testing.T) {
		req := client.FileRequest{
			Owner: "test",
			Repo:  "repo",
			Ref:   "main",
			Path:  configPath,
		}

		file, err := hybridClient.GetFile(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, string(configContent), file.Content)
	})

	t.Run("delegates to real client for source files", func(t *testing.T) {
		req := client.FileRequest{
			Owner: "test",
			Repo:  "repo",
			Ref:   "main",
			Path:  "src/main.go",
		}

		file, err := hybridClient.GetFile(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, "mock content", file.Content)
		require.Equal(t, "https://github.com/test/repo/blob/main/src/main.go", file.URL)
	})
}

type mockVCSClient struct{}

func (m *mockVCSClient) GetFile(ctx context.Context, req client.FileRequest) (client.File, error) {
	return client.File{
		Content: "mock content",
		URL:     "https://github.com/" + req.Owner + "/" + req.Repo + "/blob/" + req.Ref + "/" + req.Path,
	}, nil
}

func TestOutputSingleFunctionResults(t *testing.T) {
	results := []functionResult{
		{
			FunctionName: "testFunc",
			Path:         "/test.go",
			Covered:      true,
			ResolvedURL:  "https://github.com/test/repo/blob/main/test.go",
			SampleCount:  100,
		},
	}

	t.Run("text output", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := outputSingleFunctionResults(results)
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		output := make([]byte, 1024)
		n, _ := r.Read(output)
		outputStr := string(output[:n])

		require.Contains(t, outputStr, "Function Coverage")
		require.Contains(t, outputStr, "testFunc")
		require.Contains(t, outputStr, "/test.go")
		require.Contains(t, outputStr, "Covered:       true")
	})

}

func TestAnalyzeCoverage(t *testing.T) {
	builder := testhelper.NewProfileBuilder(1000).
		CPUProfile().
		ForStacktraceString("main", "foo").AddSamples(10)

	profileBytes, err := builder.MarshalVT()
	require.NoError(t, err)

	profile, err := pprof.RawFromBytes(profileBytes)
	require.NoError(t, err)

	cfg := &config.PyroscopeConfig{
		SourceCode: config.SourceCodeConfig{
			Mappings: []config.MappingConfig{},
		},
	}

	mockClient := &mockVCSClient{}

	functions := extractFunctions(profile.Profile)
	require.Equal(t, len(functions), 2)

	logger := log.NewNopLogger()
	httpClient := &http.Client{Timeout: 30 * time.Second}
	report := analyzeCoverage(
		context.Background(),
		profile.Profile,
		functions,
		cfg,
		mockClient,
		httpClient,
		logger,
	)

	require.Equal(t, len(functions), report.TotalFunctions)
	require.Equal(t, report.CoveredFunctions, 0)
	// No mappings in config
	require.Equal(t, report.UncoveredFunctions, 2)
	require.Equal(t, len(functions), len(report.Results))
}

func TestRunCoverageAnalysis_InvalidProfile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".pyroscope.yaml")
	profilePath := filepath.Join(tmpDir, "nonexistent.pprof")

	configContent := `source_code:
  mappings: []`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	params := &sourceCodeCoverageParams{
		ProfilePath:  profilePath,
		ConfigPath:   configPath,
		GithubToken:  "test-token",
		OutputFormat: "text",
		TopN:         0,
	}

	err = runCoverageAnalysis(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read profile")
}

func TestRunCoverageAnalysis_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".pyroscope.yaml")
	profilePath := filepath.Join(tmpDir, "test.pprof")

	// Create invalid config file
	err := os.WriteFile(configPath, []byte("invalid yaml: [["), 0644)
	require.NoError(t, err)

	builder := testhelper.NewProfileBuilder(1000).CPUProfile()
	profileBytes, err := builder.MarshalVT()
	require.NoError(t, err)
	err = os.WriteFile(profilePath, profileBytes, 0644)
	require.NoError(t, err)

	params := &sourceCodeCoverageParams{
		ProfilePath:  profilePath,
		ConfigPath:   configPath,
		GithubToken:  "test-token",
		OutputFormat: "text",
		TopN:         0,
	}

	err = runCoverageAnalysis(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse config")
}

func TestSourceCodeCoverage_ListFunctionsMode(t *testing.T) {
	builder := testhelper.NewProfileBuilder(1000).
		CPUProfile().
		ForStacktraceString("main", "foo").AddSamples(10)

	profileBytes, err := builder.MarshalVT()
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp("", "test-profile-*.pprof")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(profileBytes)
	require.NoError(t, err)
	tmpFile.Close()

	params := &sourceCodeCoverageParams{
		ProfilePath:   tmpFile.Name(),
		ListFunctions: true,
		OutputFormat:  "text",
	}

	err = sourceCodeCoverage(context.Background(), params)
	require.NoError(t, err)
}

func TestSourceCodeCoverage_ValidationErrors(t *testing.T) {
	t.Run("missing config and repo for function check", func(t *testing.T) {
		params := &sourceCodeCoverageParams{
			ProfilePath:  "test.pprof",
			FunctionName: "testFunc",
		}

		err := sourceCodeCoverage(context.Background(), params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--config is required")
	})
}

func TestOutputDetailed_ShowsErrors(t *testing.T) {
	report := &coverageReport{
		Results: []functionResult{
			{
				FunctionName: "func1",
				Path:         "/path/to/func1.go",
				Covered:      false,
				Error:        "file not found",
				SampleCount:  10,
			},
			{
				FunctionName: "func2",
				Path:         "/path/to/func2.go",
				Covered:      true,
				ResolvedURL:  "https://github.com/test/repo/blob/main/path/to/func2.go",
				SampleCount:  20,
			},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputDetailed(report)

	w.Close()
	os.Stdout = oldStdout

	output := make([]byte, 2048)
	n, _ := r.Read(output)
	outputStr := string(output[:n])

	// Errors should always be shown
	require.Contains(t, outputStr, "file not found")
	require.Contains(t, outputStr, "func1")
	require.Contains(t, outputStr, "func2")
	require.Contains(t, outputStr, "URL:")
}

func TestCheckSingleFunction_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".pyroscope.yaml")
	profilePath := filepath.Join(tmpDir, "test.pprof")

	configContent := `source_code:
  mappings: []`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	builder := testhelper.NewProfileBuilder(1000).CPUProfile()
	profileBytes, err := builder.MarshalVT()
	require.NoError(t, err)
	err = os.WriteFile(profilePath, profileBytes, 0644)
	require.NoError(t, err)

	params := &sourceCodeCoverageParams{
		ProfilePath:  profilePath,
		ConfigPath:   configPath,
		FunctionName: "nonexistentFunction",
		GithubToken:  "test-token",
		OutputFormat: "text",
	}

	err = checkSingleFunction(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no function found matching")
}

func TestExtractFunctions_EdgeCases(t *testing.T) {
	t.Run("empty profile", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{""},
			Function:    []*profilev1.Function{},
		}
		result := extractFunctions(profile)
		require.Empty(t, result)
	})

	t.Run("function with only name", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{"", "main"},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1, Filename: 0},
			},
		}
		result := extractFunctions(profile)
		require.Len(t, result, 1)
		require.Equal(t, "main", result[0].FunctionName)
		require.Equal(t, "", result[0].Path)
	})

	t.Run("function with only path", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{"", "/path/to/file.go"},
			Function: []*profilev1.Function{
				{Id: 1, Name: 0, Filename: 1},
			},
		}
		result := extractFunctions(profile)
		require.Len(t, result, 1)
		require.Equal(t, "", result[0].FunctionName)
		require.Equal(t, "/path/to/file.go", result[0].Path)
	})
}

func TestCalculateSampleCountsMap_EdgeCases(t *testing.T) {
	t.Run("empty samples", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{""},
			Function:    []*profilev1.Function{},
			Location:    []*profilev1.Location{},
			Sample:      []*profilev1.Sample{},
		}
		result := calculateSampleCountsMap(profile)
		require.Empty(t, result)
	})

	t.Run("sample with zero value", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{"", "main", "/main.go"},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1, Filename: 2},
			},
			Location: []*profilev1.Location{
				{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 10}}},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{0}}, // Zero value - should be skipped
			},
		}
		result := calculateSampleCountsMap(profile)
		require.Empty(t, result)
	})

	t.Run("multiple sample types", func(t *testing.T) {
		profile := &profilev1.Profile{
			StringTable: []string{"", "main", "/main.go"},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1, Filename: 2},
			},
			Location: []*profilev1.Location{
				{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 10}}},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{5, 10}}, // Multiple values - should sum
			},
		}
		result := calculateSampleCountsMap(profile)
		require.Equal(t, int64(15), result["main|/main.go"]) // 5 + 10
	})
}
