# Jsonnet for Grafana Pyroscope on Kubernetes

This folder contains the Jsonnet for deploying Grafana Pyroscope in Kubernetes.
The documentation for the Pyroscope Jsonnet is published at [https://grafana.com/docs/pyroscope/latest/deploy-kubernetes/tanka-jsonnet/](https://grafana.com/docs/pyroscope/latest/deploy-kubernetes/tanka-jsonnet/).

## Pre-compiled Dashboards and Rules

Ready-to-use Grafana dashboards and Prometheus recording rules compiled from the `pyroscope-mixin/` are available in [`../mixin-compiled/`](../mixin-compiled/):

- **Grafana Dashboards** - JSON files ready to import into Grafana
  - `pyroscope-reads.json` - Read path monitoring
  - `pyroscope-writes.json` - Write path monitoring
- **Prometheus Rules** - YAML files ready to load into Prometheus
  - `recording-rules.yaml` - Pre-computed metrics for performance
  - `alert-rules.yaml` - Placeholder for custom alerts

### Quick Usage

```bash
# Import dashboards into Grafana
cd ../mixin-compiled/dashboards/
# Use Grafana UI to import *.json files

# Load rules into Prometheus
cd ../mixin-compiled/rules/
# Add to your prometheus.yml rule_files section
```

### Recompiling

If you modify the mixin source files, recompile with:

```bash
cd ../../..  # Back to repo root
make compile-mixin
```

Files are automatically kept in sync via CI when mixin source files change.
