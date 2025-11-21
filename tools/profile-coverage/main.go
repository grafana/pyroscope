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
		profilePath  = flag.String("profile", "", "Path to pprof profile file")
		configPath   = flag.String("config", "", "Path to .pyroscope.yaml file")
		repoURL      = flag.String("repo", "", "GitHub repository URL (e.g., https://github.com/owner/repo)")
		ref          = flag.String("ref", "HEAD", "Git reference (default: HEAD)")
		rootPath     = flag.String("root-path", "", "Repository root path")
		githubToken  = flag.String("github-token", "", "GitHub token for API access (or use GITHUB_TOKEN env var)")
		outputFormat = flag.String("output", "all", "Output format: text, detailed, json, or all")
		verbose      = flag.Bool("verbose", false, "Show detailed error messages")
		help         = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *profilePath == "" || *configPath == "" || *repoURL == "" {
		fmt.Println("Profile Coverage Analysis Tool")
		fmt.Println()
		fmt.Println("Measures the effectiveness of .pyroscope.yaml source code mappings")
		fmt.Println("at translating function names/paths from a pprof profile to VCS source files.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s --profile <profile.pprof> --config <.pyroscope.yaml> --repo <repo-url> [flags]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Required flags:")
		flag.PrintDefaults()
		return
	}

	if err := run(*profilePath, *configPath, *repoURL, *ref, *rootPath, *githubToken, *outputFormat, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(profilePath, configPath, repoURL, ref, rootPath, githubToken, outputFormat string, verbose bool) error {
	// Read and parse config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := config.ParsePyroscopeConfig(configData)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Read profile
	profile, err := pprof.OpenFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	// Extract unique functions
	functions := extractFunctions(profile.Profile)

	// Parse repository URL
	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return fmt.Errorf("failed to parse repository URL: %w", err)
	}

	// Setup GitHub client
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
	report := analyzeCoverage(context.Background(), functions, cfg, hybridClient, gitURL, rootPath, ref, log.NewNopLogger())

	// Generate output
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

	for _, fn := range functions {
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
			// Check if it used fallback (no mapping found)
			if !result.UsedMapping {
				result.UsedFallback = true
				report.FunctionsWithFallback++
			}
		} else {
			result.Covered = true
			result.ResolvedURL = response.URL
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

