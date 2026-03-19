---
title: "Migrate from v1 to v2 storage"
menuTitle: "Migrate from v1"
description: "Step-by-step guide to migrate your Pyroscope installation from v1 to v2 storage architecture using Helm."
weight: 900
---

# Migrate from v1 to v2 storage

This guide walks you through migrating a Pyroscope installation from v1 to v2 storage architecture using the Helm chart. The migration uses a phased approach that lets you run both storage backends simultaneously before fully cutting over to v2.

For an overview of what changed in v2 and why, refer to [About the v2 architecture](../about-pyroscope-v2-architecture/) and [Design motivation](../design-motivation/).

## Prerequisites

Before starting the migration, make sure you have:

- **Helm chart version 1.15.1 or later** (the first version with v1/v2 storage support). Verify with:

  ```bash
  helm list -f pyroscope
  ```

  Check the `CHART` column shows `pyroscope-1.15.1` or higher. If your chart is older, upgrade it first.

- **Pyroscope running on v1 storage via Helm.** Verify with:

  ```bash
  helm get values pyroscope -o yaml | grep -A2 'storage:'
  ```

  You should see `v1: true` and `v2: false` (or `v2` absent, since v1 is the default). If you already see `v2: true`, your installation is already using v2 or is mid-migration.

- **Persistence enabled** (`pyroscope.persistence.enabled=true`).

- **Object storage configured.** v2 writes directly to object storage — it doesn't use local disk for block storage. If you haven't configured object storage yet, add it to your Helm values. For example, for S3:

  ```yaml
  pyroscope:
    structuredConfig:
      storage:
        backend: s3
        s3:
          endpoint: s3.us-east-1.amazonaws.com
          bucket_name: pyroscope-data
          access_key_id: "${AWS_ACCESS_KEY_ID}"
          secret_access_key: "${AWS_SECRET_ACCESS_KEY}"
  ```

  For other backends (GCS, Azure, Swift), refer to [Configure object storage backend]({{< relref "../../configure-server/storage/configure-object-storage-backend" >}}).

- **`kubectl` and `helm` CLI access** to your cluster.

## Migration overview

The migration has three phases:

| Phase | What happens | Reversible? |
|-------|-------------|-------------|
| 1. Dual ingest | v2 components deploy alongside v1. Writes go to both storage backends. | Yes, trivially |
| 2. Validate | Run both backends for at least 24 hours. Verify v2 data and compaction. | Yes, trivially |
| 3. Remove v1 | Remove v1 components. Only v2 serves reads and writes. | Partial (see [Rollback](#rollback)) |

## Phase 1: Deploy v2 components alongside v1

In this phase, you deploy the v2 components (segment-writer, metastore, compaction-worker, query-backend) alongside your existing v1 installation. The distributor starts writing to both storage backends simultaneously.

The Helm chart automatically configures:
- `write-path=combined` — the distributor sends data to both ingesters (v1) and segment-writers (v2)
- `enable-query-backend=true` — the query frontend can route reads to the v2 backend

You must also set `queryBackendFrom` to an RFC 3339 timestamp that tells the query frontend from when to start reading from the v2 backend. Set it to a time slightly in the future (a few minutes) to give the v2 components time to start up:

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true \
  --set architecture.storage.migration.queryBackendFrom=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.UTC) + datetime.timedelta(minutes=10)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
```

{{< admonition type="note" >}}
The `queryBackendFrom` timestamp tells the query frontend to use the v2 read path for data ingested after that time. Queries for older data continue to be served by the v1 read path.
{{< /admonition >}}

**Microservices mode:** If you deployed Pyroscope using the `values-micro-services.yaml` file (with individual components defined in `pyroscope.components`), you must also enable `architecture.microservices.enabled` so that the chart automatically adds the v2 components (segment-writer, metastore, compaction-worker, query-backend):

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true \
  --set architecture.microservices.enabled=true \
  --set architecture.storage.migration.queryBackendFrom=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.UTC) + datetime.timedelta(minutes=10)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
```

Your existing component overrides in `pyroscope.components` are preserved — the chart merges them on top of the default v1 and v2 component definitions.

The `--reuse-values` flag preserves your existing configuration.

### Verify Phase 1

After the upgrade completes, check that the new components are running:

```bash
# All pods should be Running
kubectl get pods -l app.kubernetes.io/instance=pyroscope

# Check the Helm notes for migration status
helm get notes pyroscope
```

You should see output confirming dual-write mode is active, with 100% of traffic going to both v1 and v2 write paths.

Also verify:

- The **metastore** raft cluster has elected a leader. Check the metastore pod logs for a message like `entering leader state`:

  ```bash
  kubectl logs -l app.kubernetes.io/component=metastore --tail=500 | grep -i "entering leader state"
  ```

  In single-binary mode, check the main pod logs instead:

  ```bash
  kubectl logs -l app.kubernetes.io/instance=pyroscope --tail=500 | grep -i "entering leader state"
  ```

- The **segment-writer** ring is healthy. Check this from the distributor, which uses the ring to route writes to segment-writers. All instances should show as `ACTIVE`:

  ```bash
  # Single-binary
  kubectl port-forward svc/pyroscope 4040:4040 &

  # Microservices
  kubectl port-forward svc/pyroscope-distributor 4040:4040 &

  curl -s http://localhost:4040/ring-segment-writer | grep -o 'ACTIVE' | wc -l
  ```

  The count should match the number of segment-writer instances.

## Phase 2: Validate v2 is working

Run both storage backends simultaneously for at least 24 hours before proceeding. During this time, verify:

### Data is being written to v2

Query recent profiling data through the Pyroscope UI or API. Data ingested after Phase 1 should be served by the v2 read path. You can use `profilecli` or the Pyroscope UI to query profiles from the last hour and confirm results are returned.

### Compaction is running

The compaction-worker compacts segments through the L0 &rarr; L1 &rarr; L2 levels. Verify that compaction jobs are completing:

```bash
# Single-binary: logs are on the main pyroscope pod
kubectl logs -l app.kubernetes.io/instance=pyroscope --tail=500 | grep "compaction finished successfully"

# Microservices: logs are on the compaction-worker pods
kubectl logs -l app.kubernetes.io/component=compaction-worker --tail=500 | grep "compaction finished successfully"
```

You should see log lines like:

```
msg="compaction finished successfully" input_blocks=20 output_blocks=1
```

Compaction typically starts within minutes of ingestion — a block is created once enough segments accumulate for a shard.

### Error rates are stable

Check that write and read error rates haven't increased since enabling v2. If you have Prometheus metrics configured, query error rates per component:

```promql
# Server-side errors by component (distributor, segment-writer, query-frontend, query-backend, etc.)
sum by (component) (rate(pyroscope_request_duration_seconds_count{status_code=~"5.."}[5m]))
```

All components should show zero or negligible error rates. Compare against pre-migration baselines to confirm no regression.

## Phase 3: Remove v1 components

Once you're confident that v2 is working correctly and you no longer need to query data ingested before Phase 1, you can remove the v1 components.

{{< admonition type="warning" >}}
After this step, data ingested before Phase 1 is no longer queryable through Pyroscope. The data still exists in object storage, but the v1 read path components (ingester, store-gateway, querier) that serve it will be removed. Make sure you don't need to query historical data from before the migration started.
{{< /admonition >}}

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true
```

**Microservices mode:** If you defined v1 components in `pyroscope.components` in your values file (for example, using the `values-micro-services.yaml` template), those definitions persist via `--reuse-values` and the chart deploys them regardless of the `v1` flag. Scale them to zero to remove them:

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true \
  --set pyroscope.components.ingester.replicaCount=0 \
  --set pyroscope.components.compactor.replicaCount=0 \
  --set pyroscope.components.store-gateway.replicaCount=0 \
  --set pyroscope.components.querier.replicaCount=0 \
  --set pyroscope.components.query-scheduler.replicaCount=0
```

### Verify Phase 3

Check that v1 components have been removed and v2 is serving all traffic:

```bash
# v1 components (ingester, store-gateway, querier, compactor) should be gone
kubectl get pods -l app.kubernetes.io/instance=pyroscope
```

Verify that queries still return data. Port-forward to the Pyroscope query endpoint and check that profile types are returned:

```bash
# Single-binary
kubectl port-forward svc/pyroscope 4040:4040 &

# Microservices
kubectl port-forward svc/pyroscope-query-frontend 4040:4040 &
```

Then open the Pyroscope UI at `http://localhost:4040` and verify that you can query recent profiles. An empty or errored UI indicates a problem — see [Rollback](#rollback).

## Rollback

### During Phase 1 or Phase 2

Rolling back is straightforward — set `architecture.storage.v2=false` to remove the v2 components and return to v1-only:

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=false
```

Data written to v2 during the dual-ingest period is orphaned but doesn't affect v1 operation.

### During or after Phase 3

If you removed v1 components (Phase 3), rolling back requires redeploying them:

```bash
helm upgrade pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true
```

This returns you to dual-ingest mode (Phase 1). If you scaled v1 components to zero in Phase 3 (microservices mode), restore their replica counts as well. Note that any data ingested between Phase 3 and the rollback was only written to v2 and won't be visible through the v1 read path.

## Helm values reference

The following Helm values control the v1/v2 storage configuration and migration behavior.

### Storage layer toggles

| Value | Type | Default | Description |
|---|---|---|---|
| `architecture.storage.v1` | bool | `true` | Enable v1 storage and its components (ingester, store-gateway, querier, compactor). |
| `architecture.storage.v2` | bool | `false` | Enable v2 storage and its components (segment-writer, metastore, compaction-worker, query-backend). |

### Migration tuning

These values only apply when both `v1` and `v2` are enabled (dual-ingest mode).

| Value | Type | Default | Description |
|---|---|---|---|
| `architecture.storage.migration.ingesterWeight` | float | `1.0` | Fraction `[0, 1]` of write traffic sent to v1 ingesters. |
| `architecture.storage.migration.segmentWriterWeight` | float | `1.0` | Fraction `[0, 1]` of write traffic sent to v2 segment-writers. |
| `architecture.storage.migration.queryBackend` | bool | `true` | Enable the v2 query backend for reads. |
| `architecture.storage.migration.queryBackendFrom` | string | `"auto"` | RFC 3339 timestamp from which the v2 read path serves traffic. Must be set to a valid timestamp (for example, `2025-01-01T00:00:00Z`). |
