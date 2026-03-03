---
name: update-go-version
description: Update Go version across the Pyroscope codebase (go.mod, go.work, CI workflows, Dockerfiles, goreleaser, examples). Use when bumping Go to a new patch or minor version.
argument-hint: <version, e.g. 1.25.7>
disable-model-invocation: true
allowed-tools: Bash, Read, Write, Edit, Grep, Glob, WebFetch
---

# Update Go Version

Updates the Go version across all relevant files in the Pyroscope codebase.

## Usage

```
/update-go-version 1.25.7
```

## Prerequisites

If no version argument is provided, do NOT guess. Instead:
1. Fetch the latest Go releases: `curl -s 'https://go.dev/dl/?mode=json' | jq -r '.[].version'`
2. Show the user the available versions and the current state (from go.mod)
3. Ask which version they want to target
4. Only proceed once they confirm

**Version validation:** The target version MUST be a full patch version (e.g. `1.25.7`), not just a minor version (e.g. `1.25` or `1.25.0`). If the user provides `X.Y.0` or `X.Y`, warn them that they should use the latest patch release for that minor (check go.dev for the latest). Using `.0` means missing security patches and will cause the `toolchain` directive to be dropped from go.mod files (Go removes it when it equals the `go` directive).

## Steps

### 1. Read current state

Extract the current versions from the codebase:
- `go` directive from `go.mod` (line 3, e.g. `go 1.24.6`)
- `toolchain` directive from `go.mod` (line 5, e.g. `toolchain go1.24.9`)
- `go-version` from `.github/workflows/ci.yml` (e.g. `1.24.13`)

Report these to the user before making changes.

### 2. Determine bump type

Compare the target version against the current `go` directive in `go.mod`:

- **Same minor** (e.g. current `go 1.25.0`, target `1.25.7`): this is a **patch bump**. Only the `toolchain` directive and build/CI files need updating. The `go` directive stays as-is.
- **Different minor** (e.g. current `go 1.24.6`, target `1.25.7`): this is a **minor bump**. Both the `go` directive and `toolchain` directive need updating, plus build/CI files.

Tell the user which type was detected before proceeding.

### 3. Update go.mod and go.work files

**Important:** Do NOT run `go mod tidy` or `go work sync` manually. Step 7 handles module synchronization correctly using `make go/mod`.

#### For a minor bump

The `go` directive sets the minimum compatible version, the `toolchain` directive sets the exact build version. They MUST be different to prevent Go from dropping the `toolchain` line.

- Set `go` directive to `X.Y.0` (the base of the new minor)
- Set `toolchain` directive to `goX.Y.Z` (the exact target patch version, which must be > X.Y.0)

Use two separate `go mod edit` calls to ensure the toolchain line is preserved:
```bash
go mod edit -go=X.Y.0 <file>
go mod edit -toolchain=goX.Y.Z <file>
```

If using a single `go mod edit -go=X.Y.0 -toolchain=goX.Y.Z` and both values are the same, Go will DROP the toolchain line. Always ensure they differ.

#### For a patch bump

Update only the `toolchain` directive to `goX.Y.Z` in all go.mod files and the root `go.work`. Do NOT change the `go` directive.

```bash
go mod edit -toolchain=goX.Y.Z <file>
```

#### Files to update

**go.mod files (all need both `go` and `toolchain` for minor bumps, only `toolchain` for patch bumps):**
- `go.mod`
- `api/go.mod`
- `lidia/go.mod`
- `examples/golang-pgo/go.mod`
- `examples/tracing/golang-push/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare-alloy/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare-k6/go.mod`
- `examples/language-sdk-instrumentation/golang-push/simple/go.mod`

**go.work files (edit directly with sed or text editing):**
- `go.work` (has both `go` and `toolchain` lines)
- `examples/golang-pgo/go.work` (only `go` line, no `toolchain`)
- `examples/tracing/golang-push/go.work` (only `go` line)
- `examples/language-sdk-instrumentation/golang-push/rideshare/go.work` (only `go` line)
- `examples/language-sdk-instrumentation/golang-push/rideshare-alloy/go.work` (only `go` line)
- `examples/language-sdk-instrumentation/golang-push/rideshare-k6/go.work` (only `go` line)
- `examples/language-sdk-instrumentation/golang-push/simple/go.work` (only `go` line)

For go.work files:
- **Minor bump**: update the `go` directive to `X.Y.0` in all go.work files, and update the `toolchain` directive to `goX.Y.Z` in the root `go.work`.
- **Patch bump**: update only the `toolchain` directive to `goX.Y.Z` in the root `go.work`. Do not touch example go.work files (they have no `toolchain` line).

### 4. Update CI workflows

Update `go-version:` in all GitHub Actions workflow files to the exact target version `X.Y.Z`. The version may be quoted or unquoted:

Files to update (check each one):
- `.github/workflows/ci.yml` (6 occurrences)
- `.github/workflows/fuzzer.yml`
- `.github/workflows/release.yml`
- `.github/workflows/test-examples.yml`
- `.github/workflows/update-contributors.yml`
- `.github/workflows/weekly-release.yml`

### 5. Update Dockerfiles

Update `FROM golang:` base images to the exact target version `X.Y.Z` in all Go example Dockerfiles.

Find them with:
```bash
git ls-files '**/Dockerfile*' | xargs grep -l 'golang:[0-9]' | grep -v ebpf/symtab/elf/testdata
```

**Do NOT touch:**
- Non-Go Dockerfiles (dotnet, java, python, nodejs — they don't use `golang:` images)
- `examples/grafana-alloy-auto-instrumentation/ebpf-otel/Dockerfile.demo` (uses `golang:1.22-alpine`, intentionally pinned to an older version)
- Any Dockerfile under `ebpf/symtab/elf/testdata/`

### 6. Update build and release configuration

- **`.goreleaser.yaml`**: Update the version check hook string:
  ```
  go version | grep "go version goX.Y.Z "
  ```

- **`.pyroscope.yaml`**: Update the Go source code ref for symbol resolution:
  ```yaml
  ref: goX.Y.Z
  ```

- **`tools/update_examples.Dockerfile`**: Update the `GO_VERSION` ARG:
  ```
  ARG GO_VERSION=X.Y.Z
  ```

### 7. Synchronize Go modules

Run the project's standard module sync target:

```bash
make go/mod
```

This runs `go work sync` and `go mod tidy` across all modules in the correct order. It is **required** because:
- CI runs `check/go/mod` which verifies modules are tidy — skipping this will fail CI
- Bumping the `go` directive changes the `go.sum` checksum retention window
- Minor version bumps may cause small, legitimate dependency adjustments (e.g. transitive minimum versions)

Review the resulting diff. Expected changes:
- `go.sum` additions/removals (checksum window shift) — **normal**
- Small indirect dependency version bumps in `go.mod` files — **normal for minor bumps**
- Large unexpected dependency changes — **investigate before committing**

### 8. Verify the build

```bash
make go/bin
```

If the build fails, investigate and fix before proceeding.

### 9. Summary

After all changes, show the user:
- Number of files modified
- Old version -> New version for each category (go directive, toolchain, CI, Dockerfiles)
- Whether it was a minor or patch bump
- Build verification result
- Summary of `make go/mod` changes (any dependency adjustments)

## Version semantics reference

| Directive | Meaning | When to update |
|-----------|---------|---------------|
| `go X.Y.0` | Minimum Go version for compatibility | Minor bumps only |
| `toolchain goX.Y.Z` | Exact build version (bug fixes, security) | Every bump (patch and minor) |
| CI `go-version` | Exact version CI uses to build/test | Every bump |
| Dockerfile `golang:X.Y.Z` | Exact version for container builds | Every bump |

## Files Reference

| Category | Files | What changes |
|----------|-------|-------------|
| Go modules | `go.mod`, `api/go.mod`, `lidia/go.mod`, `examples/**/go.mod` | `go` directive (minor bump) + `toolchain` directive (always) |
| Go workspaces | `go.work`, `examples/**/go.work` | `go` directive (minor bump) + `toolchain` directive (root only) |
| CI workflows | `.github/workflows/*.yml` | `go-version:` value |
| Dockerfiles | `examples/**/Dockerfile*` (Go ones only) | `FROM golang:` base image tag |
| Release | `.goreleaser.yaml` | Version check hook |
| Profiling | `.pyroscope.yaml` | `ref:` for Go stdlib source linking |
| Build tools | `tools/update_examples.Dockerfile` | `GO_VERSION` ARG |
