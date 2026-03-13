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

Compare the target version against the current `toolchain` directive in `go.mod`:

- **Same minor** (e.g. current `toolchain go1.25.3`, target `1.25.7`): **patch bump**. Only the `toolchain` directive and build/CI files need updating.
- **Different minor** (e.g. current `toolchain go1.24.9`, target `1.25.7`): **minor bump**. The `toolchain` directive and build/CI files need updating. Ask the user whether to also update the `go` directive (see step 4).

Tell the user which type was detected before proceeding.

### 3. Run the upgrade script

The `tools/upgrade-go-version.sh` script handles CI, Dockerfile, and release config updates. It also creates a git commit with those changes:

```bash
bash tools/upgrade-go-version.sh X.Y.Z
```

This updates and commits:
- `.github/workflows/*.yml` — `go-version:` values
- `.goreleaser.yaml` — version check hook
- `.pyroscope.yaml` — `ref:` for Go stdlib source linking and `GO_VERSION`
- `tools/update_examples.Dockerfile` — `GO_VERSION` ARG
- All Go Dockerfiles — `FROM golang:` base image tag (excluding ebpf testdata)

### 4. Update `toolchain` directive in go.mod and go.work files

Update the `toolchain` directive to `goX.Y.Z` in all go.mod files using `go mod edit`, and the root `go.work` using `go work edit`:

```bash
# For go.mod files:
go mod edit -toolchain=goX.Y.Z <file>

# For go.work files (go mod edit does NOT work on .work files):
go work edit -toolchain=goX.Y.Z <file>
```

**go.mod files:**
- `go.mod`
- `api/go.mod`
- `lidia/go.mod`
- `examples/golang-pgo/go.mod`
- `examples/tracing/golang-push/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare-alloy/go.mod`
- `examples/language-sdk-instrumentation/golang-push/rideshare-k6/go.mod`
- `examples/language-sdk-instrumentation/golang-push/simple/go.mod`

**go.work (root only — use `go work edit`):**
- `go.work`

#### Optional: update `go` directive (minor bump only)

The `go` directive sets the **minimum compatible Go version**. Only update it when:
- A dependency requires a newer Go version
- The codebase uses a Go language feature from the newer minor version
- The user explicitly requests it

If updating the `go` directive, the `go` and `toolchain` values MUST differ to prevent Go from dropping the `toolchain` line. Use two separate calls:

```bash
# For go.mod files:
go mod edit -go=X.Y.0 <file>
go mod edit -toolchain=goX.Y.Z <file>

# For go.work files (go mod edit does NOT work on .work files):
go work edit -go=X.Y.0 <file>
go work edit -toolchain=goX.Y.Z <file>
```

Also update the `go` directive in all go.work files (use `go work edit`):
- `go.work`
- `examples/golang-pgo/go.work`
- `examples/tracing/golang-push/go.work`
- `examples/language-sdk-instrumentation/golang-push/rideshare/go.work`
- `examples/language-sdk-instrumentation/golang-push/rideshare-alloy/go.work`
- `examples/language-sdk-instrumentation/golang-push/rideshare-k6/go.work`
- `examples/language-sdk-instrumentation/golang-push/simple/go.work`

### 5. Synchronize Go modules

```bash
make go/mod
```

This runs `go work sync` and `go mod tidy` across all modules. Required because CI runs `check/go/mod`.

Review the diff. Expected: `go.sum` changes, small indirect dependency bumps. Investigate anything unexpected.

### 6. Verify the build

```bash
make go/bin
```

If the build fails, investigate and fix before proceeding.

### 7. Commit remaining changes

The script already committed CI/Dockerfile/release changes. Now commit the go.mod/go.work/go.sum changes:

```bash
git add -u *.mod *.sum *.work api/ lidia/ examples/
```

Use a commit message that reflects what changed:
- Toolchain only: `"Update Go toolchain to goX.Y.Z"`
- Toolchain + go directive: `"Update Go to X.Y.Z (go directive + toolchain)"`

### 8. Summary

Show the user:
- Number of files modified
- Old -> New version for each category (go directive, toolchain, CI, Dockerfiles)
- Whether it was a minor or patch bump
- Build verification result
- Remind user to review commits and push when ready

## Version semantics reference

| Directive | Meaning | When to update |
|-----------|---------|---------------|
| `go X.Y.Z` | Minimum Go version for compatibility | Only when a dependency or language feature requires it |
| `toolchain goX.Y.Z` | Exact build version (bug fixes, security) | Every bump (patch and minor) |
| CI `go-version` | Exact version CI uses to build/test | Every bump (via script) |
| Dockerfile `golang:X.Y.Z` | Exact version for container builds | Every bump (via script) |
