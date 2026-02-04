# Update Embedded Checksums Tool

This tool automatically updates the SHA256 checksums in `pkg/embedded/grafana/grafana.go` for the embedded Grafana releases and plugins.

## Usage

Run from the repository root:

```bash
go run tools/update-embedded-checksums/main.go
```

## What it does

1. Parses `pkg/embedded/grafana/grafana.go` to extract all release artifact URLs and their checksums
2. Downloads each artifact and calculates its actual SHA256 checksum
3. Compares the actual checksums with the ones in the file
4. Updates the file with the correct checksums if any mismatches are found

## Renovate Integration

This tool is automatically run by Renovate via `postUpgradeTasks` when it detects updates to:
- Grafana releases (from `grafana/grafana`)
- grafana-pyroscope-app plugin (from `grafana/profiles-drilldown`)

When Renovate updates the version numbers in the URLs, it will automatically run this tool to fetch the correct checksums for the new versions and include them in the same PR.

## Example Output

```
Updating embedded Grafana checksums...

Processing: grafana-12.0.2.linux-amd64.tar.gz
  Current checksum: c1755b4da918edfd298d5c8d5f1ffce35982ad10e1640ec356570cfb8c34b3e8
  Downloading...
  Actual checksum:  c1755b4da918edfd298d5c8d5f1ffce35982ad10e1640ec356570cfb8c34b3e8
  ✓ Checksum matches

✓ All checksums are already correct
```
