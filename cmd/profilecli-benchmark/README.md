# ProfileCLI Benchmark Tool

A flexible benchmarking tool for testing profilecli performance with different configurations. Configure tests using YAML files and optionally upload results to Google Sheets.

## Features

- **YAML-based configuration**: Define test matrices in easy-to-read YAML files
- **Flexible test matrix**: Configure any combination of max_nodes, flags, and custom arguments
- **Multiple iterations**: Run each configuration multiple times for statistical accuracy
- **Comprehensive metrics**: Measure execution time and output size
- **Google Sheets integration**: Automatically upload results for analysis
- **CLI overrides**: Override any config value via command-line flags
- **Dry-run mode**: Test without uploading to Google Sheets

## Quick Start

### 1. Build the tools

```bash
# Build profilecli
cd /Users/christian/git/github.com/grafana/pyroscope
make go/bin

# Build benchmark tool
cd cmd/profilecli-benchmark
make build
```

### 2. Configure your benchmark

Create or edit `benchmark.yaml`:

```yaml
profilecli:
  path: "../../profilecli"

google_sheets:
  enabled: false  # Set to true and configure for Google Sheets upload

query:
  tenant_id: "test-tenant-1234"
  query: '{service_name="service-a"}'
  from: "2026-02-11T13:50:00.398Z"
  to: "2026-02-11T14:50:00.000Z"
  url: "http://localhost:4040"

tests:
  iterations: 3
  max_nodes: [0, 32, 8192, 16384]
  flags:
    - name: "baseline"
      args: []
    - name: "function-names-only"
      args: ["--function-names-only"]
```

### 3. Run the benchmark

```bash
# Run with default config (benchmark.yaml)
./profilecli-benchmark

# Run with custom config
./profilecli-benchmark --config benchmark-advanced.yaml
```

## Configuration

### YAML Structure

```yaml
profilecli:
  path: "path/to/profilecli"        # Required: Path to profilecli binary
  timeout: "5m"                      # Optional: Timeout per query

google_sheets:
  enabled: true                      # Enable/disable Google Sheets upload
  spreadsheet_id: "..."              # Your spreadsheet ID
  credentials: "path/to/creds.json"  # Google credentials file
  sheet_name: "Results"              # Sheet name (default: "ProfileCLI Benchmark Results")

query:
  tenant_id: "..."                   # Required: Tenant ID
  query: '{label="value"}'           # Required: Query selector
  from: "2026-01-01T00:00:00Z"       # Required: Start time
  to: "2026-01-02T00:00:00Z"         # Required: End time
  url: "http://localhost:4040"       # Required: Pyroscope URL
  profile_type: "..."                # Optional: Profile type

tests:
  iterations: 3                      # Number of iterations per config
  max_nodes: [0, 32, 16384]          # List of max_nodes values to test
  flags:                             # Flag configurations to test
    - name: "config-name"
      description: "Optional description"
      args: ["--flag1", "--flag2"]
  custom_args: []                    # Optional: Args for all queries
```

### Example Configurations

Three example configurations are provided:

1. **`benchmark.yaml`**: Standard configuration with balanced testing
2. **`benchmark-minimal.yaml`**: Quick testing with minimal iterations
3. **`benchmark-advanced.yaml`**: Comprehensive testing with many variations

## Command-Line Flags

The tool has a single flag:

```bash
-config string
    Path to YAML config file (default "benchmark.yaml")
```

All configuration is done via YAML files. To disable Google Sheets upload, set `google_sheets.enabled: false` in your config.

## Usage Examples

### Basic Usage

```bash
# Use default config
./profilecli-benchmark

# Use custom config
./profilecli-benchmark --config benchmark-advanced.yaml
```

### Testing Locally (No Google Sheets)

```bash
# Use a config with google_sheets.enabled=false
./profilecli-benchmark --config benchmark-minimal.yaml
```

## Output

### Console Output

Real-time progress and summary statistics:

```
Starting benchmark tests...
Configuration: 4 max_nodes × 2 flag configs × 3 iterations = 24 total tests

[1/24] Running test: max_nodes=0, flags=baseline, iteration=1
  ✓ Completed in 1.234s (output size: 45.2 MB)
[2/24] Running test: max_nodes=0, flags=baseline, iteration=2
  ✓ Completed in 1.198s (output size: 45.1 MB)
...

================================================================================
BENCHMARK SUMMARY
================================================================================

Config: max_nodes=0, flags=baseline
  Runs: 3 successful, 0 failed
  Duration - Min: 1.198s, Avg: 1.220s, Max: 1.234s
  Output Size - Avg: 45.2 MB

Config: max_nodes=32, flags=baseline
  Runs: 3 successful, 0 failed
  Duration - Min: 987ms, Avg: 1.001s, Max: 1.015s
  Output Size - Avg: 2.1 MB
...

--------------------------------------------------------------------------------
OVERALL: 24 successful, 0 failed, 24 total
================================================================================
```

### Google Sheets Output

Results are appended to the specified sheet with columns:

| Timestamp | Config Name | Max Nodes | Flag Config | Iteration | Duration (ms) | Duration (seconds) | Output Size (bytes) | Output Size (formatted) | Success | Error |
|-----------|-------------|-----------|-------------|-----------|---------------|-------------------|---------------------|------------------------|---------|-------|
| 2026-02-13 10:30:45 | max_nodes=0,flags=baseline | 0 | baseline | 1 | 1234 | 1.234 | 47392819 | 45.2 MB | true | |

## Google Sheets Setup

If you want to upload results to Google Sheets, follow the setup guide in `SETUP.md`.

Quick summary:
1. Create a Google Cloud Project
2. Enable Google Sheets API
3. Create a service account and download credentials
4. Create a spreadsheet and share it with the service account
5. Update your YAML config with spreadsheet ID and credentials path

## Customizing Tests

### Adding More max_nodes Values

Edit your YAML config:

```yaml
tests:
  max_nodes: [0, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768]
```

### Adding Custom Flag Configurations

```yaml
tests:
  flags:
    - name: "baseline"
      description: "Standard query"
      args: []

    - name: "function-names-only"
      description: "Faster, less detail"
      args:
        - "--function-names-only"

    - name: "with-selector"
      description: "With stacktrace filtering"
      args:
        - "--stacktrace-selector"
        - "main"

    - name: "memory-profile"
      description: "Memory profile instead of CPU"
      args:
        - "--profile-type"
        - "memory:inuse_space:bytes:space:bytes"
```

### Adding Global Custom Arguments

Arguments to add to every query:

```yaml
tests:
  custom_args:
    - "--verbose"
    - "--collect-diagnostics"
```

## Makefile Targets

```bash
make build          # Build the benchmark tool
make clean          # Remove build artifacts
make run-dry-run    # Run with dry-run mode
make init           # Download and tidy Go modules
make help           # Show help
```

## Troubleshooting

### "Failed to load config"

- Check that the config file exists
- Verify YAML syntax (use a YAML validator)
- Check file permissions

### "profilecli not found"

```bash
# Build profilecli first
cd /Users/christian/git/github.com/grafana/pyroscope
make go/bin
```

### "Invalid configuration"

- Ensure all required fields are set
- Check that `tests.iterations > 0`
- Verify that `tests.max_nodes` and `tests.flags` have at least one item

### Query failures

- Verify Pyroscope server is running
- Check tenant ID exists and has data for the time range
- Test the query manually with profilecli:
  ```bash
  ./profilecli query profile \
    --url "http://localhost:4040" \
    --tenant-id "test" \
    --query '{service_name="app"}' \
    --from "2026-01-01T00:00:00Z" \
    --to "2026-01-02T00:00:00Z"
  ```

### Google Sheets upload fails

- Follow the complete setup guide in `SETUP.md`
- Verify service account has access to the spreadsheet
- Check credentials file path is correct
- Test with `--dry-run` first

## Performance Tips

1. **Start small**: Use `benchmark-minimal.yaml` to test your setup
2. **Use timeouts**: Set `profilecli.timeout` to prevent hanging queries
3. **Iterate gradually**: Start with `iterations: 1`, then increase
4. **Monitor resources**: Large queries can consume significant memory
5. **Test incrementally**: Add max_nodes values gradually to find limits

## Advanced Usage

### Multiple Config Files

Test different scenarios:

```bash
# Test memory profiles
./profilecli-benchmark --config configs/memory-benchmark.yaml

# Test CPU profiles
./profilecli-benchmark --config configs/cpu-benchmark.yaml

# Test different time ranges
./profilecli-benchmark --config configs/24h-benchmark.yaml
```

### Automated Testing

```bash
#!/bin/bash
# Run benchmarks and save logs

configs=("benchmark-minimal.yaml" "benchmark.yaml" "benchmark-advanced.yaml")

for config in "${configs[@]}"; do
    echo "Running benchmark with $config"
    ./profilecli-benchmark --config "$config" 2>&1 | tee "logs/${config%.yaml}-$(date +%Y%m%d-%H%M%S).log"
done
```

### Continuous Benchmarking

Set up a cron job or CI/CD pipeline:

```bash
# Daily benchmark at 2 AM
0 2 * * * cd /path/to/profilecli-benchmark && ./profilecli-benchmark --config daily-benchmark.yaml
```

## Example Workflows

### Development Workflow

```bash
# 1. Quick smoke test
./profilecli-benchmark --config benchmark-minimal.yaml --dry-run

# 2. Full local test
./profilecli-benchmark --config benchmark.yaml --dry-run

# 3. Upload results
./profilecli-benchmark --config benchmark.yaml
```

### Performance Regression Testing

```bash
# Benchmark before changes
./profilecli-benchmark --config benchmark.yaml --sheet-name "Before Changes"

# Make code changes...

# Benchmark after changes
./profilecli-benchmark --config benchmark.yaml --sheet-name "After Changes"

# Compare results in Google Sheets
```

## Contributing

To add new features or fix bugs:

1. Modify `main.go`
2. Update documentation
3. Test with `make build && ./profilecli-benchmark --dry-run`
4. Update example YAML configs if needed

## License

Same as the parent Pyroscope project.
