package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"gopkg.in/yaml.v3"
)

// Config represents the full benchmark configuration from YAML
type Config struct {
	ProfileCLI   ProfileCLIConfig   `yaml:"profilecli"`
	GoogleSheets GoogleSheetsConfig `yaml:"google_sheets"`
	Queries      []QueryConfig      `yaml:"queries"`
	Tests        TestsConfig        `yaml:"tests"`
}

type ProfileCLIConfig struct {
	Path    string `yaml:"path"`
	Timeout string `yaml:"timeout,omitempty"` // e.g., "5m"
}

type GoogleSheetsConfig struct {
	Enabled       bool   `yaml:"enabled"`
	SpreadsheetID string `yaml:"spreadsheet_id"`
	Credentials   string `yaml:"credentials"`
	SheetName     string `yaml:"sheet_name"`
}

type QueryConfig struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	TenantID    string `yaml:"tenant_id"`
	Query       string `yaml:"query"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	URL         string `yaml:"url"`
	ProfileType string `yaml:"profile_type,omitempty"`
}

type TestsConfig struct {
	Iterations  int              `yaml:"iterations"`
	MaxNodes    []int64          `yaml:"max_nodes"`
	Flags       []FlagConfig     `yaml:"flags"`
	ProfileType string           `yaml:"profile_type,omitempty"`
	CustomArgs  []string         `yaml:"custom_args,omitempty"`
}

type FlagConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Args        []string `yaml:"args,omitempty"`
}

type TestResult struct {
	QueryName      string
	ConfigName     string
	MaxNodes       int64
	FlagName       string
	Iteration      int
	Duration       time.Duration
	Success        bool
	Error          string
	OutputSize     int64
	SampleTypeUnit string
	TotalValue     int64
	NumSamples     int64
	DiagnosticsID  string
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

var (
	configFile = flag.String("config", "benchmark.yaml", "Path to YAML config file")
)

func main() {
	flag.Parse()

	// Load configuration from YAML
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Run tests
	results := runBenchmarks(config)

	// Print summary
	printSummary(config, results)

	// Upload to Google Sheets if enabled
	if config.GoogleSheets.Enabled {
		log.Println("\nUploading results to Google Sheets...")
		if err := uploadToSheets(config.GoogleSheets, results); err != nil {
			log.Fatalf("Failed to upload results: %v", err)
		}

		// Upload comparison summary
		if err := uploadComparisonToSheets(config, results); err != nil {
			log.Printf("Warning: Failed to upload comparison summary: %v", err)
		}

		log.Printf("Results uploaded successfully to: https://docs.google.com/spreadsheets/d/%s",
			config.GoogleSheets.SpreadsheetID)
	} else {
		log.Println("\nGoogle Sheets upload disabled in config")
	}
}

func expandHomedir(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		return path
	}

	if path == "~" {
		return usr.HomeDir
	}

	return filepath.Join(usr.HomeDir, path[2:])
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Apply defaults
	if config.ProfileCLI.Path == "" {
		config.ProfileCLI.Path = "profilecli"
	}
	if config.ProfileCLI.Timeout == "" {
		config.ProfileCLI.Timeout = "30m"
	}

	// Expand home directory in paths
	config.GoogleSheets.Credentials = expandHomedir(config.GoogleSheets.Credentials)

	return &config, nil
}

func validateConfig(config *Config) error {
	if config.GoogleSheets.Enabled {
		if config.GoogleSheets.SpreadsheetID == "" {
			return fmt.Errorf("google_sheets.spreadsheet_id is required when Google Sheets is enabled")
		}
		if config.GoogleSheets.Credentials == "" {
			return fmt.Errorf("google_sheets.credentials is required when Google Sheets is enabled")
		}
	}

	if len(config.Queries) == 0 {
		return fmt.Errorf("queries must have at least one query configuration")
	}

	// Validate each query
	for i, q := range config.Queries {
		if q.TenantID == "" {
			return fmt.Errorf("query[%d]: tenant_id is required", i)
		}
		if q.Query == "" {
			return fmt.Errorf("query[%d]: query is required", i)
		}
		if q.From == "" {
			return fmt.Errorf("query[%d]: from is required", i)
		}
		if q.To == "" {
			return fmt.Errorf("query[%d]: to is required", i)
		}
		if q.URL == "" {
			return fmt.Errorf("query[%d]: url is required", i)
		}
		// Set default name if not provided
		if q.Name == "" {
			config.Queries[i].Name = fmt.Sprintf("query-%d", i+1)
		}
	}

	if config.Tests.Iterations <= 0 {
		return fmt.Errorf("tests.iterations must be greater than 0")
	}

	if len(config.Tests.MaxNodes) == 0 {
		return fmt.Errorf("tests.max_nodes must have at least one value")
	}

	if len(config.Tests.Flags) == 0 {
		return fmt.Errorf("tests.flags must have at least one configuration")
	}

	return nil
}

func runBenchmarks(config *Config) []TestResult {
	var results []TestResult

	totalTests := len(config.Queries) * len(config.Tests.MaxNodes) * len(config.Tests.Flags) * config.Tests.Iterations
	currentTest := 0

	log.Println("Starting benchmark tests...")
	log.Printf("Configuration: %d queries × %d max_nodes × %d flag configs × %d iterations = %d total tests\n",
		len(config.Queries), len(config.Tests.MaxNodes), len(config.Tests.Flags), config.Tests.Iterations, totalTests)

	for _, queryConfig := range config.Queries {
		log.Printf("\n%s--- Running tests for query: %s ---%s", colorCyan, queryConfig.Name, colorReset)
		if queryConfig.Description != "" {
			log.Printf("    Description: %s", queryConfig.Description)
		}
		log.Printf("    Query: %s", queryConfig.Query)
		log.Printf("    Time range: %s to %s\n", queryConfig.From, queryConfig.To)

		for _, maxNodes := range config.Tests.MaxNodes {
			for _, flagConfig := range config.Tests.Flags {
				for i := 0; i < config.Tests.Iterations; i++ {
					currentTest++
					log.Printf("[%d/%d] Running test: query=%s, max_nodes=%d, flags=%s, iteration=%d",
						currentTest, totalTests, queryConfig.Name, maxNodes, flagConfig.Name, i+1)

					result := runTest(config, queryConfig, maxNodes, flagConfig, i+1)
					results = append(results, result)

					if result.Success {
						diagMsg := ""
						if result.DiagnosticsID != "" {
							diagMsg = fmt.Sprintf(", diag_id: %s", result.DiagnosticsID)
						}
						log.Printf("  %s✓%s Completed in %v (size: %s, samples: %s, total: %d %s%s)",
							colorGreen,
							colorReset,
							result.Duration,
							formatBytes(result.OutputSize),
							formatNumber(result.NumSamples),
							result.TotalValue,
							result.SampleTypeUnit,
							diagMsg)
					} else {
						log.Printf("  %s✗%s Failed: %s", colorRed, colorReset, result.Error)
					}
				}
			}
		}
	}

	return results
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}

func median(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Sort a copy
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func runTest(config *Config, queryConfig QueryConfig, maxNodes int64, flagConfig FlagConfig, iteration int) TestResult {
	result := TestResult{
		QueryName:  queryConfig.Name,
		MaxNodes:   maxNodes,
		FlagName:   flagConfig.Name,
		Iteration:  iteration,
		ConfigName: fmt.Sprintf("query=%s,max_nodes=%d,flags=%s", queryConfig.Name, maxNodes, flagConfig.Name),
	}

	// Create temp file for pprof output
	tmpFile, err := os.CreateTemp("", "profilecli-bench-*.pprof")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		return result
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Build command arguments
	args := []string{
		"query", "profile",
		"--url", queryConfig.URL,
		"--tenant-id", queryConfig.TenantID,
		"--query", queryConfig.Query,
		"--from", queryConfig.From,
		"--to", queryConfig.To,
		"--output", fmt.Sprintf("pprof=%s", tmpPath),
		"--force", // Overwrite if exists
	}

	// Add profile type if specified
	profileType := queryConfig.ProfileType
	if profileType == "" {
		profileType = config.Tests.ProfileType
	}
	if profileType != "" {
		args = append(args, "--profile-type", profileType)
	}

	// Add max-nodes if specified
	if maxNodes > 0 {
		args = append(args, "--max-nodes", fmt.Sprintf("%d", maxNodes))
	}

	// Add flag-specific arguments
	if len(flagConfig.Args) > 0 {
		args = append(args, flagConfig.Args...)
	}

	// Add custom arguments from tests config
	if len(config.Tests.CustomArgs) > 0 {
		args = append(args, config.Tests.CustomArgs...)
	}

	// Resolve profilecli path
	// If it's just a command name (no path separators), look it up in PATH
	// Otherwise, treat it as a file path and make it absolute
	var cmdPath string
	if !strings.Contains(config.ProfileCLI.Path, string(filepath.Separator)) {
		// It's just a command name, look it up in PATH
		foundPath, err := exec.LookPath(config.ProfileCLI.Path)
		if err != nil {
			result.Error = fmt.Sprintf("profilecli not found in PATH: %v", err)
			return result
		}
		cmdPath = foundPath
	} else {
		// It's a file path, make it absolute
		absPath, err := filepath.Abs(config.ProfileCLI.Path)
		if err != nil {
			result.Error = fmt.Sprintf("failed to get absolute path: %v", err)
			return result
		}
		// Check if profilecli exists
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			result.Error = fmt.Sprintf("profilecli not found at: %s", absPath)
			return result
		}
		cmdPath = absPath
	}

	// Run the command and measure time
	cmd := exec.Command(cmdPath, args...)

	// Set timeout if specified
	var ctx context.Context
	var cancel context.CancelFunc
	if config.ProfileCLI.Timeout != "" {
		timeout, err := time.ParseDuration(config.ProfileCLI.Timeout)
		if err != nil {
			result.Error = fmt.Sprintf("invalid timeout: %v", err)
			return result
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, cmdPath, args...)
	}

	startTime := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	result.Duration = duration

	// Extract diagnostics ID from output if present
	result.DiagnosticsID = extractDiagnosticsID(string(output))

	if err != nil {
		// Check if it was a timeout
		if ctx != nil && ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("timeout exceeded (%s)", config.ProfileCLI.Timeout)
		} else {
			result.Error = fmt.Sprintf("%v\nOutput: %s", err, string(output))
		}
		result.Success = false
		return result
	}

	// Parse the pprof file
	if err := parsePprofMetrics(tmpPath, &result); err != nil {
		result.Error = fmt.Sprintf("failed to parse pprof: %v", err)
		result.Success = false
		return result
	}

	result.Success = true
	return result
}

func extractDiagnosticsID(output string) string {
	// Look for diagnostics_id in the output
	// Format: level=info msg="query diagnostics" diagnostics_id=<id>
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "diagnostics_id") {
			// Extract the diagnostics_id value
			parts := strings.Split(line, "diagnostics_id=")
			if len(parts) > 1 {
				// Get the ID (may be quoted or not)
				id := strings.TrimSpace(parts[1])
				// Remove quotes if present
				id = strings.Trim(id, "\"")
				// Take only up to the first space (in case there are more fields)
				if idx := strings.Index(id, " "); idx > 0 {
					id = id[:idx]
				}
				return id
			}
		}
	}
	return ""
}

func parsePprofMetrics(pprofPath string, result *TestResult) error {
	// Open and parse the pprof file
	f, err := os.Open(pprofPath)
	if err != nil {
		return fmt.Errorf("failed to open pprof file: %w", err)
	}
	defer f.Close()

	// Get file size
	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat pprof file: %w", err)
	}
	result.OutputSize = stat.Size()

	// Parse the pprof using google/pprof
	prof, err := profile.Parse(f)
	if err != nil {
		return fmt.Errorf("failed to parse pprof: %w", err)
	}

	// Extract metrics from the first sample type
	if len(prof.SampleType) == 0 {
		return fmt.Errorf("no sample types in profile")
	}

	// Get the first sample type info
	firstSampleType := prof.SampleType[0]
	result.SampleTypeUnit = fmt.Sprintf("%s:%s", firstSampleType.Type, firstSampleType.Unit)

	// Calculate total value and number of samples
	var totalValue int64
	for _, sample := range prof.Sample {
		if len(sample.Value) > 0 {
			totalValue += sample.Value[0]
		}
	}

	result.TotalValue = totalValue
	result.NumSamples = int64(len(prof.Sample))

	return nil
}

func printSummary(config *Config, results []TestResult) {
	log.Println("\n" + strings.Repeat("=", 80))
	log.Printf("%sBENCHMARK SUMMARY%s", colorYellow, colorReset)
	log.Println(strings.Repeat("=", 80))

	// Group results by configuration
	type configKey struct {
		QueryName string
		MaxNodes  int64
		FlagName  string
	}

	grouped := make(map[configKey][]time.Duration)
	outputSizes := make(map[configKey][]int64)
	numSamples := make(map[configKey][]int64)
	totalValues := make(map[configKey][]int64)
	sampleTypeUnits := make(map[configKey]string)
	diagnosticsIDs := make(map[configKey][]string)
	failed := make(map[configKey]int)

	for _, r := range results {
		key := configKey{
			QueryName: r.QueryName,
			MaxNodes:  r.MaxNodes,
			FlagName:  r.FlagName,
		}
		if r.Success {
			grouped[key] = append(grouped[key], r.Duration)
			outputSizes[key] = append(outputSizes[key], r.OutputSize)
			numSamples[key] = append(numSamples[key], r.NumSamples)
			totalValues[key] = append(totalValues[key], r.TotalValue)
			if r.DiagnosticsID != "" {
				diagnosticsIDs[key] = append(diagnosticsIDs[key], r.DiagnosticsID)
			}
			if sampleTypeUnits[key] == "" {
				sampleTypeUnits[key] = r.SampleTypeUnit
			}
		} else {
			failed[key]++
		}
	}

	// Get baseline flag name (first flag in config)
	baselineFlagName := ""
	if len(config.Tests.Flags) > 0 {
		baselineFlagName = config.Tests.Flags[0].Name
	}

	// Group by query for better organization
	byQuery := make(map[string][]configKey)
	for key := range grouped {
		byQuery[key.QueryName] = append(byQuery[key.QueryName], key)
	}

	// Print statistics for each query
	for queryName, keys := range byQuery {
		log.Printf("\n%s=== Query: %s ===%s", colorCyan, queryName, colorReset)

		for _, key := range keys {
			durations := grouped[key]
			if len(durations) == 0 {
				continue
			}

			var totalSize int64
			var totalSamples int64
			var totalVal int64
			minDuration := durations[0]
			maxDuration := durations[0]

			for i, d := range durations {
				totalSize += outputSizes[key][i]
				totalSamples += numSamples[key][i]
				totalVal += totalValues[key][i]
				if d < minDuration {
					minDuration = d
				}
				if d > maxDuration {
					maxDuration = d
				}
			}

			medianDuration := median(durations)
			avgSize := totalSize / int64(len(outputSizes[key]))
			avgSamples := totalSamples / int64(len(numSamples[key]))
			avgTotal := totalVal / int64(len(totalValues[key]))

			log.Printf("\n  Config: max_nodes=%d, flags=%s", key.MaxNodes, key.FlagName)
			log.Printf("    Runs: %d successful, %d failed", len(durations), failed[key])
			log.Printf("    Duration - Min: %v, Median: %v, Max: %v", minDuration, medianDuration, maxDuration)
			log.Printf("    Output Size - Avg: %s", formatBytes(avgSize))
			log.Printf("    Samples - Avg: %s", formatNumber(avgSamples))
			log.Printf("    Total Value - Avg: %d %s", avgTotal, sampleTypeUnits[key])
			if len(diagnosticsIDs[key]) > 0 {
				log.Printf("    Diagnostics IDs: %s", strings.Join(diagnosticsIDs[key], ", "))
			}
		}
	}

	// Print relative performance comparison
	log.Println("\n" + strings.Repeat("=", 80))
	log.Printf("%sRELATIVE PERFORMANCE COMPARISON%s", colorYellow, colorReset)
	log.Println(strings.Repeat("=", 80))

	// Group by query and max_nodes for comparison
	type compareKey struct {
		QueryName string
		MaxNodes  int64
	}

	comparisons := make(map[compareKey]map[string]time.Duration) // compareKey -> flagName -> medianDuration

	for key, durations := range grouped {
		if len(durations) == 0 {
			continue
		}

		medianDuration := median(durations)

		cmpKey := compareKey{
			QueryName: key.QueryName,
			MaxNodes:  key.MaxNodes,
		}

		if comparisons[cmpKey] == nil {
			comparisons[cmpKey] = make(map[string]time.Duration)
		}
		comparisons[cmpKey][key.FlagName] = medianDuration
	}

	// Print comparisons
	for queryName := range byQuery {
		log.Printf("\n%s=== Query: %s ===%s", colorCyan, queryName, colorReset)

		// Get all max_nodes for this query
		maxNodesSet := make(map[int64]bool)
		for cmpKey := range comparisons {
			if cmpKey.QueryName == queryName {
				maxNodesSet[cmpKey.MaxNodes] = true
			}
		}

		// Sort max_nodes
		var maxNodesList []int64
		for mn := range maxNodesSet {
			maxNodesList = append(maxNodesList, mn)
		}
		sort.Slice(maxNodesList, func(i, j int) bool {
			return maxNodesList[i] < maxNodesList[j]
		})

		for _, maxNodes := range maxNodesList {
			cmpKey := compareKey{
				QueryName: queryName,
				MaxNodes:  maxNodes,
			}

			flagDurations := comparisons[cmpKey]
			if len(flagDurations) < 2 {
				continue // Need at least 2 configs to compare
			}

			// Use first flag as baseline
			baselineDuration := flagDurations[baselineFlagName]
			if baselineDuration == 0 {
				// Baseline flag not found for this config, skip comparison
				continue
			}

			log.Printf("\n  max_nodes=%d (baseline: %s at %v):", maxNodes, baselineFlagName, baselineDuration)

			// Use flag order from config for consistent output
			for _, flagConfig := range config.Tests.Flags {
				flagName := flagConfig.Name
				duration, exists := flagDurations[flagName]
				if !exists {
					continue
				}

				if flagName == baselineFlagName {
					log.Printf("    %s%s: %v (baseline - 100%%)%s", colorCyan, flagName, duration, colorReset)
				} else {
					ratio := float64(duration) / float64(baselineDuration)
					percentDiff := (ratio - 1.0) * 100

					if ratio > 1.0 {
						// Slower - red
						log.Printf("    %s%s: %v (%.1f%% slower)%s", colorRed, flagName, duration, percentDiff, colorReset)
					} else {
						// Faster - green
						log.Printf("    %s%s: %v (%.1f%% faster)%s", colorGreen, flagName, duration, -percentDiff, colorReset)
					}
				}
			}
		}
	}

	// Print overall summary
	totalSuccess := 0
	totalFailed := 0
	for _, durations := range grouped {
		totalSuccess += len(durations)
	}
	for _, count := range failed {
		totalFailed += count
	}

	log.Println("\n" + strings.Repeat("-", 80))
	log.Printf("OVERALL: %d successful, %d failed, %d total",
		totalSuccess, totalFailed, totalSuccess+totalFailed)
	log.Println(strings.Repeat("=", 80))
}

func uploadToSheets(gsConfig GoogleSheetsConfig, results []TestResult) error {
	ctx := context.Background()

	// Create Google Sheets service
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile(gsConfig.Credentials))
	if err != nil {
		return fmt.Errorf("unable to create sheets client: %w", err)
	}

	// Prepare header row
	header := []interface{}{
		"Timestamp",
		"Query Name",
		"Config Name",
		"Max Nodes",
		"Flag Config",
		"Iteration",
		"Duration (ms)",
		"Duration (seconds)",
		"Output Size (bytes)",
		"Output Size (formatted)",
		"Sample Type:Unit",
		"Num Samples",
		"Total Value",
		"Diagnostics ID",
		"Success",
		"Error",
	}

	// Prepare data rows
	var rows [][]interface{}
	rows = append(rows, header)

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	for _, r := range results {
		row := []interface{}{
			timestamp,
			r.QueryName,
			r.ConfigName,
			r.MaxNodes,
			r.FlagName,
			r.Iteration,
			r.Duration.Milliseconds(),
			fmt.Sprintf("%.3f", r.Duration.Seconds()),
			r.OutputSize,
			formatBytes(r.OutputSize),
			r.SampleTypeUnit,
			r.NumSamples,
			r.TotalValue,
			r.DiagnosticsID,
			r.Success,
			r.Error,
		}
		rows = append(rows, row)
	}

	// Create or get sheet
	spreadsheet, err := srv.Spreadsheets.Get(gsConfig.SpreadsheetID).Do()
	if err != nil {
		return fmt.Errorf("unable to get spreadsheet: %w", err)
	}

	// Check if sheet exists
	var sheetID int64
	sheetExists := false
	sheetName := gsConfig.SheetName
	if sheetName == "" {
		sheetName = "ProfileCLI Benchmark Results"
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			sheetID = sheet.Properties.SheetId
			sheetExists = true
			break
		}
	}

	// Create sheet if it doesn't exist
	if !sheetExists {
		req := &sheets.Request{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{
					Title: sheetName,
				},
			},
		}

		batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{req},
		}

		resp, err := srv.Spreadsheets.BatchUpdate(gsConfig.SpreadsheetID, batchUpdateRequest).Do()
		if err != nil {
			return fmt.Errorf("unable to create sheet: %w", err)
		}
		sheetID = resp.Replies[0].AddSheet.Properties.SheetId
	}

	// Append data
	valueRange := &sheets.ValueRange{
		Values: rows,
	}

	rangeStr := fmt.Sprintf("%s!A1", sheetName)
	_, err = srv.Spreadsheets.Values.Append(gsConfig.SpreadsheetID, rangeStr, valueRange).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Do()

	if err != nil {
		return fmt.Errorf("unable to append data: %w", err)
	}

	// Format header row (make it bold)
	requests := []*sheets.Request{
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:       sheetID,
					StartRowIndex: 0,
					EndRowIndex:   1,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold: true,
						},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		},
	}

	batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(gsConfig.SpreadsheetID, batchUpdateRequest).Do()
	if err != nil {
		// Non-fatal error
		log.Printf("Warning: failed to format header: %v", err)
	}

	return nil
}
