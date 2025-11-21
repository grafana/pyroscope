package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log"
	giturl "github.com/kubescape/go-git-url"
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
		return client.File{
			Content: string(c.configContent),
			URL:     fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", req.Owner, req.Repo, req.Ref, req.Path),
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
	UsedMapping  bool
	UsedFallback bool
}

type coverageReport struct {
	TotalFunctions      int
	CoveredFunctions     int
	UncoveredFunctions   int
	CoveragePercentage  float64
	FunctionsWithMapping int
	FunctionsWithFallback int
	Results             []functionResult
}

func main() {
	var (
		profilePath   = flag.String("profile", "", "Path to pprof profile file")
		configPath    = flag.String("config", "", "Path to .pyroscope.yaml file")
		repoURL       = flag.String("repo", "", "GitHub repository URL (e.g., https://github.com/owner/repo)")
		ref           = flag.String("ref", "HEAD", "Git reference (default: HEAD)")
		rootPath      = flag.String("root-path", "", "Repository root path")
		githubToken   = flag.String("github-token", "", "GitHub token for API access (or use GITHUB_TOKEN env var)")
		outputFormat  = flag.String("output", "all", "Output format: text, detailed, json, or all")
		verbose       = flag.Bool("verbose", false, "Show detailed error messages")
		listFunctions = flag.Bool("list-functions", false, "List all functions in the profile and exit")
		functionName  = flag.String("function", "", "Check coverage for a specific function (by name or path)")
		help          = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		fmt.Println("Profile Coverage Analysis Tool")
		fmt.Println()
		fmt.Println("Measures the effectiveness of .pyroscope.yaml source code mappings")
		fmt.Println("at translating function names/paths from a pprof profile to VCS source files.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s --profile <profile.pprof> [--list-functions | --function <name> | --config <.pyroscope.yaml> --repo <repo-url>]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Modes:")
		fmt.Println("  --list-functions    List all functions in the profile and exit")
		fmt.Println("  --function <name>   Check coverage for a specific function")
		fmt.Println("  (default)           Analyze coverage for all functions")
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	if *profilePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --profile is required\n")
		os.Exit(1)
	}

	// List functions mode
	if *listFunctions {
		if err := listAllFunctions(*profilePath, *outputFormat); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Single function check mode
	if *functionName != "" {
		if *configPath == "" || *repoURL == "" {
			fmt.Fprintf(os.Stderr, "Error: --config and --repo are required when using --function\n")
			os.Exit(1)
		}
		if err := checkSingleFunction(*profilePath, *configPath, *repoURL, *ref, *rootPath, *githubToken, *functionName, *outputFormat, *verbose); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Full coverage analysis mode
	if *configPath == "" || *repoURL == "" {
		fmt.Fprintf(os.Stderr, "Error: --config and --repo are required for full coverage analysis\n")
		os.Exit(1)
	}

	if err := run(*profilePath, *configPath, *repoURL, *ref, *rootPath, *githubToken, *outputFormat, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(profilePath, configPath, repoURL, ref, rootPath, githubToken, outputFormat string, verbose bool) error {
	// Read and parse config
	fmt.Fprintf(os.Stderr, "Reading configuration from %s...\n", configPath)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := config.ParsePyroscopeConfig(configData)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Loaded configuration with %d mapping(s)\n", len(cfg.SourceCode.Mappings))

	// Read profile
	fmt.Fprintf(os.Stderr, "Reading profile from %s...\n", profilePath)
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	// Extract unique functions
	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	functions := extractFunctions(profile.Profile)
	fmt.Fprintf(os.Stderr, "✓ Found %d unique function(s)\n", len(functions))

	// Parse repository URL
	fmt.Fprintf(os.Stderr, "Parsing repository URL...\n")
	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repository URL: %w", err)
	}

	// Setup GitHub client
	fmt.Fprintf(os.Stderr, "Setting up GitHub client...\n")
	tokenStr := githubToken
	if tokenStr == "" {
		tokenStr = os.Getenv("GITHUB_TOKEN")
	}
	if tokenStr == "" {
		return fmt.Errorf("GitHub token required (use --github-token flag or GITHUB_TOKEN env var)")
	}

	token := &oauth2.Token{AccessToken: tokenStr}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ghClient, err := client.GithubClient(context.Background(), token, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ GitHub client ready\n")

	// Create hybrid client
	configPathInRepo := config.PyroscopeConfigPath
	if rootPath != "" {
		configPathInRepo = filepath.Join(rootPath, config.PyroscopeConfigPath)
	}
	hybridClient := &hybridVCSClient{
		configContent: configData,
		configPath:    configPathInRepo,
		realClient:    ghClient,
	}

	// Analyze coverage
	fmt.Fprintf(os.Stderr, "\nAnalyzing coverage (this may take a while)...\n")
	report := analyzeCoverage(context.Background(), functions, cfg, hybridClient, gitURL, rootPath, ref, log.NewNopLogger())

	// Generate output
	fmt.Fprintf(os.Stderr, "\nGenerating report...\n")
	return generateOutput(report, outputFormat, verbose)
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

func analyzeCoverage(ctx context.Context, functions []config.FileSpec, cfg *config.PyroscopeConfig, vcsClient source.VCSClient, repo giturl.IGitURL, rootPath, ref string, logger log.Logger) *coverageReport {
	report := &coverageReport{
		TotalFunctions: len(functions),
		Results:        make([]functionResult, 0, len(functions)),
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	total := len(functions)
	for i, fn := range functions {
		// Show progress
		fmt.Fprintf(os.Stderr, "Processing function %d/%d: %s", i+1, total, fn.FunctionName)
		if fn.Path != "" {
			fmt.Fprintf(os.Stderr, " (%s)", fn.Path)
		}
		fmt.Fprintf(os.Stderr, "... ")

		result := functionResult{
			FunctionName: fn.FunctionName,
			Path:         fn.Path,
		}

		// Check if there's a mapping
		mapping := cfg.FindMapping(fn)
		result.UsedMapping = mapping != nil

		// Create FileFinder
		finder := source.NewFileFinder(
			vcsClient,
			repo,
			fn,
			rootPath,
			ref,
			httpClient,
			logger,
		)

		// Try to resolve the file
		response, err := finder.Find(ctx)
		if err != nil {
			result.Covered = false
			result.Error = err.Error()
			fmt.Fprintf(os.Stderr, "✗\n")
			// Check if it used fallback (no mapping found)
			if !result.UsedMapping {
				result.UsedFallback = true
				report.FunctionsWithFallback++
			}
		} else {
			result.Covered = true
			result.ResolvedURL = response.URL
			fmt.Fprintf(os.Stderr, "✓\n")
			report.CoveredFunctions++
			if result.UsedMapping {
				report.FunctionsWithMapping++
			} else {
				result.UsedFallback = true
				report.FunctionsWithFallback++
			}
		}

		report.Results = append(report.Results, result)
	}

	report.UncoveredFunctions = report.TotalFunctions - report.CoveredFunctions
	if report.TotalFunctions > 0 {
		report.CoveragePercentage = float64(report.CoveredFunctions) / float64(report.TotalFunctions) * 100
	}

	fmt.Fprintf(os.Stderr, "\n✓ Analysis complete: %d/%d functions covered (%.2f%%)\n", 
		report.CoveredFunctions, report.TotalFunctions, report.CoveragePercentage)

	return report
}

func generateOutput(report *coverageReport, format string, verbose bool) error {
	formats := []string{format}
	if format == "all" {
		formats = []string{"text", "detailed", "json"}
	}

	for _, f := range formats {
		switch f {
		case "text":
			outputText(report)
		case "detailed":
			outputDetailed(report, verbose)
		case "json":
			if err := outputJSON(report); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown output format: %s", f)
		}
	}

	return nil
}

func outputText(report *coverageReport) {
	fmt.Println("=== Coverage Summary ===")
	fmt.Printf("Total Functions:     %d\n", report.TotalFunctions)
	fmt.Printf("Covered Functions:   %d\n", report.CoveredFunctions)
	fmt.Printf("Uncovered Functions: %d\n", report.UncoveredFunctions)
	fmt.Printf("Coverage:            %.2f%%\n", report.CoveragePercentage)
	fmt.Printf("Functions with Mapping: %d\n", report.FunctionsWithMapping)
	fmt.Printf("Functions with Fallback: %d\n", report.FunctionsWithFallback)
	fmt.Println()
}

func outputDetailed(report *coverageReport, verbose bool) {
	fmt.Println("=== Detailed Results ===")
	fmt.Println()

	covered := []functionResult{}
	uncovered := []functionResult{}

	for _, result := range report.Results {
		if result.Covered {
			covered = append(covered, result)
		} else {
			uncovered = append(uncovered, result)
		}
	}

	if len(covered) > 0 {
		fmt.Printf("Covered Functions (%d):\n", len(covered))
		for _, result := range covered {
			fmt.Printf("  ✓ %s\n", result.FunctionName)
			if result.Path != "" {
				fmt.Printf("    Path: %s\n", result.Path)
			}
			if result.ResolvedURL != "" {
				fmt.Printf("    URL: %s\n", result.ResolvedURL)
			}
			if result.UsedMapping {
				fmt.Printf("    Used mapping: yes\n")
			} else if result.UsedFallback {
				fmt.Printf("    Used fallback: yes\n")
			}
			fmt.Println()
		}
	}

	if len(uncovered) > 0 {
		fmt.Printf("Uncovered Functions (%d):\n", len(uncovered))
		for _, result := range uncovered {
			fmt.Printf("  ✗ %s\n", result.FunctionName)
			if result.Path != "" {
				fmt.Printf("    Path: %s\n", result.Path)
			}
			if verbose && result.Error != "" {
				fmt.Printf("    Error: %s\n", result.Error)
			}
			if !result.UsedMapping {
				fmt.Printf("    No mapping found\n")
			}
			fmt.Println()
		}
	}
}

func outputJSON(report *coverageReport) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func listAllFunctions(profilePath, outputFormat string) error {
	fmt.Fprintf(os.Stderr, "Reading profile from %s...\n", profilePath)
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	functions := extractFunctions(profile.Profile)
	fmt.Fprintf(os.Stderr, "✓ Found %d unique function(s)\n\n", len(functions))

	switch outputFormat {
	case "json":
		type functionList struct {
			TotalFunctions int                `json:"total_functions"`
			Functions      []config.FileSpec `json:"functions"`
		}
		list := functionList{
			TotalFunctions: len(functions),
			Functions:      functions,
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(list)
	default:
		fmt.Println("=== Functions in Profile ===")
		fmt.Printf("Total: %d\n\n", len(functions))
		for i, fn := range functions {
			fmt.Printf("%d. Function: %s\n", i+1, fn.FunctionName)
			if fn.Path != "" {
				fmt.Printf("   Path: %s\n", fn.Path)
			}
			fmt.Println()
		}
	}

	return nil
}

func checkSingleFunction(profilePath, configPath, repoURL, ref, rootPath, githubToken, functionName, outputFormat string, verbose bool) error {
	// Read and parse config
	fmt.Fprintf(os.Stderr, "Reading configuration from %s...\n", configPath)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := config.ParsePyroscopeConfig(configData)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Loaded configuration with %d mapping(s)\n", len(cfg.SourceCode.Mappings))

	// Read profile
	fmt.Fprintf(os.Stderr, "Reading profile from %s...\n", profilePath)
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	// Extract all functions
	fmt.Fprintf(os.Stderr, "Extracting functions from profile...\n")
	allFunctions := extractFunctions(profile.Profile)

	// Find matching function(s)
	var matchingFunctions []config.FileSpec
	for _, fn := range allFunctions {
		if fn.FunctionName == functionName || fn.Path == functionName ||
			strings.Contains(fn.FunctionName, functionName) ||
			strings.Contains(fn.Path, functionName) {
			matchingFunctions = append(matchingFunctions, fn)
		}
	}

	if len(matchingFunctions) == 0 {
		return fmt.Errorf("no function found matching: %s", functionName)
	}

	if len(matchingFunctions) > 1 {
		fmt.Fprintf(os.Stderr, "⚠ Found %d matching functions, checking all of them...\n\n", len(matchingFunctions))
	}

	// Parse repository URL
	fmt.Fprintf(os.Stderr, "Parsing repository URL...\n")
	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repository URL: %w", err)
	}

	// Setup GitHub client
	fmt.Fprintf(os.Stderr, "Setting up GitHub client...\n")
	tokenStr := githubToken
	if tokenStr == "" {
		tokenStr = os.Getenv("GITHUB_TOKEN")
	}
	if tokenStr == "" {
		return fmt.Errorf("GitHub token required (use --github-token flag or GITHUB_TOKEN env var)")
	}

	token := &oauth2.Token{AccessToken: tokenStr}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ghClient, err := client.GithubClient(context.Background(), token, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ GitHub client ready\n")

	// Create hybrid client
	configPathInRepo := config.PyroscopeConfigPath
	if rootPath != "" {
		configPathInRepo = filepath.Join(rootPath, config.PyroscopeConfigPath)
	}
	hybridClient := &hybridVCSClient{
		configContent: configData,
		configPath:    configPathInRepo,
		realClient:    ghClient,
	}

	// Check coverage for matching functions
	fmt.Fprintf(os.Stderr, "\nChecking coverage for function(s)...\n")
	results := make([]functionResult, 0, len(matchingFunctions))
	ctx := context.Background()
	logger := log.NewNopLogger()

	for i, fn := range matchingFunctions {
		if len(matchingFunctions) > 1 {
			fmt.Fprintf(os.Stderr, "\n[%d/%d] ", i+1, len(matchingFunctions))
		}
		fmt.Fprintf(os.Stderr, "Function: %s", fn.FunctionName)
		if fn.Path != "" {
			fmt.Fprintf(os.Stderr, " (Path: %s)", fn.Path)
		}
		fmt.Fprintf(os.Stderr, "... ")

		result := functionResult{
			FunctionName: fn.FunctionName,
			Path:         fn.Path,
		}

		// Check if there's a mapping
		mapping := cfg.FindMapping(fn)
		result.UsedMapping = mapping != nil

		// Create FileFinder
		finder := source.NewFileFinder(
			hybridClient,
			gitURL,
			fn,
			rootPath,
			ref,
			httpClient,
			logger,
		)

		// Try to resolve the file
		response, err := finder.Find(ctx)
		if err != nil {
			result.Covered = false
			result.Error = err.Error()
			fmt.Fprintf(os.Stderr, "✗\n")
		} else {
			result.Covered = true
			result.ResolvedURL = response.URL
			fmt.Fprintf(os.Stderr, "✓\n")
		}

		if !result.UsedMapping {
			result.UsedFallback = true
		}

		results = append(results, result)
	}

	// Generate output
	fmt.Fprintf(os.Stderr, "\nGenerating report...\n\n")
	return outputSingleFunctionResults(results, outputFormat, verbose)
}

func outputSingleFunctionResults(results []functionResult, format string, verbose bool) error {
	switch format {
	case "json":
		type singleFunctionReport struct {
			Functions []functionResult `json:"functions"`
		}
		report := singleFunctionReport{Functions: results}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	default:
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
				if verbose && result.Error != "" {
					fmt.Printf("Error:         %s\n", result.Error)
				}
			}
			fmt.Printf("Used Mapping:  %v\n", result.UsedMapping)
			if result.UsedFallback {
				fmt.Printf("Used Fallback: %v\n", result.UsedFallback)
			}
			if i < len(results)-1 {
				fmt.Println()
			}
		}
		return nil
	}
}

