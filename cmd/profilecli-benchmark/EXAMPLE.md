# Example Configurations

## Using profilecli from PATH

The simplest configuration - just omit the `profilecli` section entirely:

```yaml
# No profilecli section needed - uses "profilecli" from PATH

google_sheets:
  enabled: false

queries:
  - name: "my-query"
    tenant_id: "my-tenant"
    query: '{service_name="app"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"

tests:
  iterations: 3
  max_nodes: [0, 1024]
  flags:
    - name: "baseline"
      args: []
```

## Using relative or absolute path

If profilecli is not in PATH, specify the path:

```yaml
profilecli:
  path: "../../profilecli"          # Relative path
  # OR
  # path: "/usr/local/bin/profilecli"  # Absolute path

queries:
  - name: "my-query"
    # ... rest of config
```

## Testing multiple queries

Compare different time ranges or queries:

```yaml
queries:
  - name: "last-hour"
    tenant_id: "prod"
    query: '{service_name="api"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"

  - name: "last-day"
    tenant_id: "prod"
    query: '{service_name="api"}'
    from: "2026-02-10T14:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"

  - name: "different-service"
    tenant_id: "prod"
    query: '{service_name="worker"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"

tests:
  iterations: 3
  max_nodes: [0, 1024, 8192]
  flags:
    - name: "baseline"
      args: []
```

This will run: 3 queries × 3 max_nodes × 1 flag × 3 iterations = **27 tests**

## Testing different profile types

```yaml
queries:
  - name: "cpu-profile"
    tenant_id: "prod"
    query: '{service_name="api"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"
    profile_type: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"

  - name: "memory-profile"
    tenant_id: "prod"
    query: '{service_name="api"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"
    profile_type: "memory:inuse_space:bytes:space:bytes"

tests:
  iterations: 5
  max_nodes: [0, 32, 1024, 8192]
  flags:
    - name: "baseline"
      args: []
    - name: "function-names"
      args: ["--function-names-only"]
```

## Testing different flag configurations

```yaml
queries:
  - name: "my-query"
    tenant_id: "prod"
    query: '{service_name="api"}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T14:00:00Z"
    url: "http://localhost:4040"

tests:
  iterations: 3
  max_nodes: [0, 8192]

  flags:
    - name: "reference"
      description: "Standard pprof format"
      args: []

    - name: "tree-format"
      description: "Tree-based pprof"
      args: ["--some-tree-flag"]

    - name: "with-diagnostics"
      description: "With diagnostics enabled"
      args: ["--collect-diagnostics"]

    - name: "function-names"
      description: "Fast, function names only"
      args: ["--function-names-only"]
```

## Using different config files

Use different YAML files for different scenarios:

```bash
# Test with different configurations
./profilecli-benchmark --config cpu-benchmark.yaml
./profilecli-benchmark --config memory-benchmark.yaml
./profilecli-benchmark --config quick-test.yaml
```

## Minimal quick test

Perfect for smoke testing:

```yaml
google_sheets:
  enabled: false

queries:
  - name: "quick"
    tenant_id: "test"
    query: '{}'
    from: "2026-02-11T13:00:00Z"
    to: "2026-02-11T13:05:00Z"  # Just 5 minutes
    url: "http://localhost:4040"

tests:
  iterations: 1              # Single run
  max_nodes: [0]            # Just one config
  flags:
    - name: "baseline"
      args: []
```

Run with: `./profilecli-benchmark --config quick-test.yaml`
