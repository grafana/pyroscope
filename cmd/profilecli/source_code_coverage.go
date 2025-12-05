package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/client"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
	"github.com/grafana/pyroscope/pkg/frontend/vcs/source"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type hybridVCSClient struct {
	configContent []byte
	configPath    string
	realClient    source.VCSClient
}

func (c *hybridVCSClient) GetFile(ctx context.Context, req client.FileRequest) (client.File, error) {
	// Intercept .pyroscope.yaml requests
	// Check if this is a request for the config file
	if req.Path == c.configPath ||
		req.Path == config.PyroscopeConfigPath ||
		strings.HasSuffix(req.Path, ".pyroscope.yaml") ||
		strings.HasSuffix(req.Path, "/.pyroscope.yaml") {
		// Don't need real url since this is for config file
		url := req.Path
		return client.File{
			Content: string(c.configContent),
			URL:     url,
		}, nil
	}
	// Delegate to real client for actual source files
	return c.realClient.GetFile(ctx, req)
}

type functionResult struct {
	FunctionName string
	Path         string
	Covered      bool
	Error        string
	ResolvedURL  string
	SampleCount  int64
}

type coverageReport struct {
	TotalFunctions     int
	CoveredFunctions   int
	UncoveredFunctions int
	CoveragePercentage float64
	Results            []functionResult
}

type sourceCodeCoverageParams struct {
	ProfilePath   string
	ConfigPath    string
	GithubToken   string
	OutputFormat  string
	ListFunctions bool
	FunctionName  string
	TopN          int
}

func addSourceCodeCoverageParams(cmd commander) *sourceCodeCoverageParams {
	params := new(sourceCodeCoverageParams)
	cmd.Flag("profile", "Path to pprof profile file").Required().StringVar(&params.ProfilePath)
	cmd.Flag("config", "Path to .pyroscope.yaml file").StringVar(&params.ConfigPath)
	cmd.Flag("output", "Output format: text or detailed").Default("text").StringVar(&params.OutputFormat)
	cmd.Flag("list-functions", "List all functions in the profile and exit").BoolVar(&params.ListFunctions)
	cmd.Flag("function", "Check coverage for a specific function (by name or path)").StringVar(&params.FunctionName)
	cmd.Flag("top", "Only process the top N functions by sample count (0 = process all)").Default("0").IntVar(&params.TopN)
	cmd.Flag("github-token", "GitHub token for API access").Envar(envPrefix + "GITHUB_TOKEN").StringVar(&params.GithubToken)
	return params
}

func sourceCodeCoverage(ctx context.Context, params *sourceCodeCoverageParams) error {
	// List functions mode
	if params.ListFunctions {
		return listAllFunctions(params.ProfilePath)
	}

	// Single function check mode
	if params.FunctionName != "" {
		if params.ConfigPath == "" {
			return errors.New("--config is required when using --function")
		}
		return checkSingleFunction(ctx, params)
	}

	// Full coverage analysis mode
	if params.ConfigPath == "" {
		return errors.New("--config is required for full coverage analysis")
	}

	return runCoverageAnalysis(ctx, params)
}

func loadConfigAndProfile(configPath, profilePath string) (*config.PyroscopeConfig, []byte, *pprof.Profile, error) {
	fmt.Fprintf(os.Stderr, "Reading configuration from %s...\n", configPath)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to read config file")
	}

	cfg, err := config.ParsePyroscopeConfig(configData)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to parse config")
	}
	fmt.Fprintf(os.Stderr, "✓ Loaded configuration with %d mapping(s)\n", len(cfg.SourceCode.Mappings))

	fmt.Fprintf(os.Stderr, "Reading profile from %s...\n", profilePath)
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to read profile")
	}

	return cfg, configData, profile, nil
}

func setupVCSClient(ctx context.Context, configData []byte, githubToken string) (source.VCSClient, *http.Client, error) {
	fmt.Fprintf(os.Stderr, "Setting up GitHub client...\n")
	if githubToken == "" {
		return nil, nil, errors.New("GitHub token required (use --github-token flag or PROFILECLI_GITHUB_TOKEN env var)")
	}

	token := &oauth2.Token{AccessToken: githubToken}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ghClient, err := client.GithubClient(ctx, token, httpClient)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create GitHub client")
	}
	fmt.Fprintf(os.Stderr, "✓ GitHub client ready\n")

	configPathInRepo := config.PyroscopeConfigPath
	vcsClient := &hybridVCSClient{
		configContent: configData,
		configPath:    configPathInRepo,
		realClient:    ghClient,
	}

	return vcsClient, httpClient, nil
}

func checkFunctionCoverage(ctx context.Context, fn config.FileSpec, cfg *config.PyroscopeConfig, vcsClient source.VCSClient, httpClient *http.Client, logger log.Logger) functionResult {
	result := functionResult{
		FunctionName: fn.FunctionName,
		Path:         fn.Path,
	}

	mapping := cfg.FindMapping(fn)

	if mapping == nil {
		result.Covered = false
		result.Error = "no mapping found"
	} else {
		dummyRepo, _ := giturl.NewGitURL("https://github.com/dummy/repo")

		finder := source.NewFileFinder(
			vcsClient,
			dummyRepo,
			fn,
			"",
			"",
			httpClient,
			logger,
		)

		response, err := finder.Find(ctx)
		if err != nil {
			result.Covered = false
			result.Error = err.Error()
		} else {
			result.Covered = true
			result.ResolvedURL = response.URL
		}
	}

	return result
}

func runCoverageAnalysis(ctx context.Context, params *sourceCodeCoverageParams) error {
	cfg, configData, profile, err := loadConfigAndProfile(params.ConfigPath, params.ProfilePath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	functions := extractFunctions(profile.Profile)
	fmt.Fprintf(os.Stderr, "✓ Found %d unique function(s)\n", len(functions))

	fmt.Fprintf(os.Stderr, "Calculating sample counts and sorting functions...\n")
	sampleCounts := calculateSampleCountsMap(profile.Profile)

	type funcWithCount struct {
		fn    config.FileSpec
		count int64
	}
	funcsWithCounts := make([]funcWithCount, 0, len(functions))
	for _, fn := range functions {
		key := fmt.Sprintf("%s|%s", fn.FunctionName, fn.Path)
		count := sampleCounts[key]
		funcsWithCounts = append(funcsWithCounts, funcWithCount{fn: fn, count: count})
	}

	// Sort by sample count in descending order
	sort.Slice(funcsWithCounts, func(i, j int) bool {
		return funcsWithCounts[i].count > funcsWithCounts[j].count
	})

	if params.TopN > 0 && params.TopN < len(funcsWithCounts) {
		funcsWithCounts = funcsWithCounts[:params.TopN]
		fmt.Fprintf(os.Stderr, "✓ Filtered to top %d functions by sample count\n", len(funcsWithCounts))
	} else {
		fmt.Fprintf(os.Stderr, "✓ Sorted %d functions by sample count\n", len(funcsWithCounts))
	}

	functions = make([]config.FileSpec, len(funcsWithCounts))
	for i, fwc := range funcsWithCounts {
		functions[i] = fwc.fn
	}

	vcsClient, httpClient, err := setupVCSClient(ctx, configData, params.GithubToken)
	if err != nil {
		return err
	}

	logger := log.NewNopLogger()
	fmt.Fprintf(os.Stderr, "\nAnalyzing coverage (this may take a while)...\n")
	report := analyzeCoverage(ctx, profile.Profile, functions, cfg, vcsClient, httpClient, logger)

	fmt.Fprintf(os.Stderr, "\nGenerating report...\n")
	return generateOutput(report, params.OutputFormat)
}

func extractFunctions(profile *profilev1.Profile) []config.FileSpec {
	seen := make(map[string]bool)
	var functions []config.FileSpec

	for _, fn := range profile.Function {
		var functionName, filePath string

		if fn.Name > 0 && int(fn.Name) < len(profile.StringTable) {
			functionName = profile.StringTable[fn.Name]
		}
		if fn.Filename > 0 && int(fn.Filename) < len(profile.StringTable) {
			filePath = profile.StringTable[fn.Filename]
		}

		// Skip functions with no name or path
		if functionName == "" && filePath == "" {
			continue
		}

		// Create a unique key for this function
		key := fmt.Sprintf("%s|%s", functionName, filePath)
		if seen[key] {
			continue
		}
		seen[key] = true

		functions = append(functions, config.FileSpec{
			FunctionName: functionName,
			Path:         filePath,
		})
	}

	return functions
}

func analyzeCoverage(ctx context.Context, profile *profilev1.Profile, functions []config.FileSpec, cfg *config.PyroscopeConfig, vcsClient source.VCSClient, httpClient *http.Client, logger log.Logger) *coverageReport {
	report := &coverageReport{
		TotalFunctions: len(functions),
		Results:        make([]functionResult, 0, len(functions)),
	}

	functionSampleCounts := calculateSampleCountsMap(profile)

	total := len(functions)
	for i, fn := range functions {
		key := fmt.Sprintf("%s|%s", fn.FunctionName, fn.Path)
		sampleCount := functionSampleCounts[key]

		fmt.Fprintf(os.Stderr, "Processing function %d/%d: %s", i+1, total, fn.FunctionName)
		if fn.Path != "" {
			fmt.Fprintf(os.Stderr, " (%s)", fn.Path)
		}
		fmt.Fprintf(os.Stderr, " (samples: %d)... ", sampleCount)

		result := checkFunctionCoverage(ctx, fn, cfg, vcsClient, httpClient, logger)
		result.SampleCount = sampleCount

		if result.Covered {
			fmt.Fprintf(os.Stderr, "✓\n")
			report.CoveredFunctions++
		} else {
			fmt.Fprintf(os.Stderr, "✗\n")
		}

		report.Results = append(report.Results, result)
	}

	report.UncoveredFunctions = report.TotalFunctions - report.CoveredFunctions
	if report.TotalFunctions > 0 {
		report.CoveragePercentage = float64(report.CoveredFunctions) / float64(report.TotalFunctions) * 100
	}

	report.sortBySampleCount()

	fmt.Fprintf(os.Stderr, "\n✓ Analysis complete: %d/%d functions covered (%.2f%%)\n",
		report.CoveredFunctions, report.TotalFunctions, report.CoveragePercentage)

	return report
}

func calculateSampleCountsMap(profile *profilev1.Profile) map[string]int64 {
	functionSampleCounts := make(map[string]int64)

	// Build maps for efficient lookup by ID (IDs are 1-indexed and may not be sequential)
	locationMap := make(map[uint64]*profilev1.Location)
	for _, loc := range profile.Location {
		locationMap[loc.Id] = loc
	}

	functionMap := make(map[uint64]*profilev1.Function)
	for _, fn := range profile.Function {
		functionMap[fn.Id] = fn
	}

	// Process each sample in the profile
	for _, sample := range profile.Sample {
		// Sum all sample values (there can be multiple sample types)
		var sampleValue int64
		for _, value := range sample.Value {
			sampleValue += value
		}

		if sampleValue == 0 {
			continue
		}

		// Count samples for each function in the stack
		seenFunctions := make(map[string]bool)
		for _, locationID := range sample.LocationId {
			location, ok := locationMap[locationID]
			if !ok {
				continue
			}
			for _, line := range location.Line {
				if line.FunctionId == 0 {
					continue
				}
				fn, ok := functionMap[line.FunctionId]
				if !ok {
					continue
				}

				// Extract function name and path
				var functionName, filePath string
				if fn.Name > 0 && int(fn.Name) < len(profile.StringTable) {
					functionName = profile.StringTable[fn.Name]
				}
				if fn.Filename > 0 && int(fn.Filename) < len(profile.StringTable) {
					filePath = profile.StringTable[fn.Filename]
				}

				// Use function key to avoid double counting in the same sample
				key := fmt.Sprintf("%s|%s", functionName, filePath)
				if !seenFunctions[key] {
					functionSampleCounts[key] += sampleValue
					seenFunctions[key] = true
				}
			}
		}
	}

	return functionSampleCounts
}

func (r *coverageReport) sortBySampleCount() {
	sort.Slice(r.Results, func(i, j int) bool {
		return r.Results[i].SampleCount > r.Results[j].SampleCount
	})
}

func generateOutput(report *coverageReport, format string) error {
	switch format {
	case "text":
		outputText(report)
	case "detailed":
		outputDetailed(report)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}

	return nil
}

func outputText(report *coverageReport) {
	fmt.Println("=== Coverage Summary ===")
	fmt.Printf("Total Functions:     %d\n", report.TotalFunctions)
	fmt.Printf("Covered Functions:   %d\n", report.CoveredFunctions)
	fmt.Printf("Uncovered Functions: %d\n", report.UncoveredFunctions)
	fmt.Printf("Coverage:            %.2f%%\n", report.CoveragePercentage)
	fmt.Println()
}

func outputDetailed(report *coverageReport) {
	fmt.Println("=== Detailed Results (ordered by sample count) ===")
	fmt.Println()

	// Results are already sorted by sample count in descending order
	for _, result := range report.Results {
		if result.Covered {
			fmt.Printf("  ✓ %s", result.FunctionName)
		} else {
			fmt.Printf("  ✗ %s", result.FunctionName)
		}
		fmt.Printf(" (samples: %d)\n", result.SampleCount)
		if result.Path != "" {
			fmt.Printf("    Path: %s\n", result.Path)
		}
		if result.Covered {
			if result.ResolvedURL != "" {
				fmt.Printf("    URL: %s\n", result.ResolvedURL)
			}
		} else {
			if result.Error != "" {
				fmt.Printf("    Error: %s\n", result.Error)
			}
			if result.Error == "no mapping found" {
				fmt.Printf("    No mapping found\n")
			}
		}
		fmt.Println()
	}
}

func listAllFunctions(profilePath string) error {
	fmt.Fprintf(os.Stderr, "Reading profile from %s...\n", profilePath)
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read profile")
	}

	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	functions := extractFunctions(profile.Profile)
	fmt.Fprintf(os.Stderr, "✓ Found %d unique function(s)\n\n", len(functions))

	fmt.Println("=== Functions in Profile ===")
	fmt.Printf("Total: %d\n\n", len(functions))
	for i, fn := range functions {
		fmt.Printf("%d. Function: %s\n", i+1, fn.FunctionName)
		if fn.Path != "" {
			fmt.Printf("   Path: %s\n", fn.Path)
		}
		fmt.Println()
	}

	return nil
}

func checkSingleFunction(ctx context.Context, params *sourceCodeCoverageParams) error {
	cfg, configData, profile, err := loadConfigAndProfile(params.ConfigPath, params.ProfilePath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	allFunctions := extractFunctions(profile.Profile)

	var matchingFunctions []config.FileSpec
	for _, fn := range allFunctions {
		if fn.FunctionName == params.FunctionName || fn.Path == params.FunctionName ||
			strings.Contains(fn.FunctionName, params.FunctionName) ||
			strings.Contains(fn.Path, params.FunctionName) {
			matchingFunctions = append(matchingFunctions, fn)
		}
	}

	if len(matchingFunctions) == 0 {
		return fmt.Errorf("no function found matching: %s", params.FunctionName)
	}

	if len(matchingFunctions) > 1 {
		fmt.Fprintf(os.Stderr, "⚠ Found %d matching functions, checking all of them...\n\n", len(matchingFunctions))
	}

	vcsClient, httpClient, err := setupVCSClient(ctx, configData, params.GithubToken)
	if err != nil {
		return err
	}

	logger := log.NewNopLogger()
	fmt.Fprintf(os.Stderr, "\nChecking coverage for function(s)...\n")
	results := make([]functionResult, 0, len(matchingFunctions))

	for i, fn := range matchingFunctions {
		if len(matchingFunctions) > 1 {
			fmt.Fprintf(os.Stderr, "\n[%d/%d] ", i+1, len(matchingFunctions))
		}
		fmt.Fprintf(os.Stderr, "Function: %s", fn.FunctionName)
		if fn.Path != "" {
			fmt.Fprintf(os.Stderr, " (Path: %s)", fn.Path)
		}
		fmt.Fprintf(os.Stderr, "... ")

		result := checkFunctionCoverage(ctx, fn, cfg, vcsClient, httpClient, logger)
		if result.Covered {
			fmt.Fprintf(os.Stderr, "✓\n")
		} else {
			fmt.Fprintf(os.Stderr, "✗\n")
		}

		results = append(results, result)
	}

	fmt.Fprintf(os.Stderr, "\nGenerating report...\n\n")
	return outputSingleFunctionResults(results)
}

func outputSingleFunctionResults(results []functionResult) error {
	for i, result := range results {
		if len(results) > 1 {
			fmt.Printf("=== Function %d ===\n", i+1)
		} else {
			fmt.Println("=== Function Coverage ===")
		}
		fmt.Printf("Function Name: %s\n", result.FunctionName)
		if result.Path != "" {
			fmt.Printf("Path:          %s\n", result.Path)
		}
		fmt.Printf("Covered:       %v\n", result.Covered)
		if result.Covered {
			fmt.Printf("Resolved URL:  %s\n", result.ResolvedURL)
		} else {
			if result.Error != "" {
				fmt.Printf("Error:         %s\n", result.Error)
			}
		}
		if i < len(results)-1 {
			fmt.Println()
		}
	}
	return nil
}
