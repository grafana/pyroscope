---
title: "Migrate from v1 to v2 storage using Helm"
menuTitle: "Migrate from v1 using Helm"
description: "Step-by-step guide to migrate your Pyroscope Helm installation from v1 to v2 storage architecture."
weight: 900
---

# Migrate from v1 to v2 storage using Helm

This guide walks you through migrating a Pyroscope installation from v1 to v2 storage architecture using the Helm chart. The migration uses a phased approach that lets you run both storage backends simultaneously before fully cutting over to v2.

For an overview of what changed in v2 and why, refer to [About the v2 architecture](../about-pyroscope-v2-architecture/) and [Design motivation](../design-motivation/).

## Prerequisites

Before starting the migration, make sure you have:

- **Helm chart version 1.19.2 or later**. Verify with:

  ```bash
  helm list -n pyroscope -f pyroscope
  ```

  Check that the `CHART` column shows `pyroscope-1.19.2` or higher. If your chart is older, upgrade it first.

- **Pyroscope running on v1 storage via Helm.** Verify with:

  ```bash
  helm get values -n pyroscope pyroscope -o yaml --all | grep -A8 'storage:' | grep -E 'v1:|v2:'
  ```

  You should see `v1: true` and `v2: false`. If you see `v2: true`, your installation is already using v2 or is mid-migration.

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

  For other backends (GCS, Azure, Swift), refer to [Configure object storage backend](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/storage/configure-object-storage-backend/). You can also use the `filesystem` backend, but in Kubernetes, this requires a `ReadWriteMany` volume that is shared across all pods.

- **`kubectl` and `helm` CLI access** to your cluster.

{{< admonition type="note" >}}
The examples in this guide assume Pyroscope is installed in the `pyroscope` namespace with the release name `pyroscope`. Adjust the `-n` namespace flag and release name in `helm` and `kubectl` commands if your installation differs.
{{< /admonition >}}

## Migration overview

The migration has three phases:

| Phase                    | What happens                                                            | Reversible? |
|--------------------------|-------------------------------------------------------------------------|-------------|
| 1.&nbsp;Dual&nbsp;ingest | v2 components deploy alongside v1. Writes go to both storage backends.  | Yes         |
| 2.&nbsp;Validate         | Run both backends for at least 24 hours. Verify v2 data and compaction. | Yes         |
| 3.&nbsp;Remove&nbsp;v1   | Remove v1 components. Only v2 serves reads and writes.                  | Partial     |

The steps below are specific to your deployment mode. Follow the section that matches your installation.

## Single-binary mode

### Phase 1: Enable dual ingest

In this phase, the single-binary process enables the v2 storage modules alongside v1. Writes go to both storage backends simultaneously, and the read path serves data from both v1 and v2.

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true
```

The `--reuse-values` flag preserves your existing configuration. Alternatively, you can pass your values file with `-f values.yaml`.

#### Verify Phase 1

After the upgrade completes, check that the pod has restarted and is running:

```bash
kubectl get pods -n pyroscope -l app.kubernetes.io/instance=pyroscope
```

Check the Helm release notes for migration status:

```bash
helm get notes -n pyroscope pyroscope
```

You should see output similar to:

```
# Pyroscope v2 Migration is active

Write traffic will be written to:
- 100% v1: ingester
- 100% v2: segment-writer

Read traffic is served from v2 read path from as soon as data was first ingested to v2.
```

Also verify:

- The **metastore** raft has initialized. Check the pod logs for a message like `entering leader state`:

  ```bash
  kubectl logs -n pyroscope -l app.kubernetes.io/instance=pyroscope --tail=500 | grep -i "entering leader state"
  ```

- The **segment-writer** ring is healthy:

  ```bash
  kubectl port-forward -n pyroscope svc/pyroscope 4040:4040 &
  PF_PID=$!
  sleep 2
  curl -s http://localhost:4040/ring-segment-writer | grep -o 'ACTIVE' | wc -l
  kill $PF_PID
  ```

  In single-binary mode, the count should be 1.

### Phase 2: Validate v2 is working

Run both storage backends simultaneously for at least 24 hours before proceeding. During this time, you should be able to query data ingested to v2.

#### Verify data is being written to v2

Query recent profiling data. The v2 read path should serve data ingested after Phase 1. You can use `profilecli`, the Pyroscope UI, or the API to query profiles from the last hour and confirm results are returned:

```bash
kubectl port-forward -n pyroscope svc/pyroscope 4040:4040 &
PF_PID=$!
sleep 2
profilecli query series --url http://localhost:4040 --from "now-1h" --to "now"
kill $PF_PID
```

You should see series labels for the profiling data being ingested. If no results are returned, check the distributor and segment-writer logs for errors.

#### Verify v2 compaction is running

The compaction-worker compacts segments through the L0 &rarr; L1 &rarr; L2 levels. Verify that compaction jobs are completing:

```bash
kubectl logs -n pyroscope -l app.kubernetes.io/instance=pyroscope --tail=500 | grep "compaction finished successfully"
```

You should see log lines like:

```
msg="compaction finished successfully" input_blocks=20 output_blocks=1
```

Compaction typically starts within minutes of ingestion, the first block is created once enough segments accumulate for a shard.

#### Verify error rates are stable

Check that write and read error rates haven't increased since enabling v2. If you have Prometheus metrics configured:

```promql
sum(rate(pyroscope_request_duration_seconds_count{status_code=~"5.."}[5m]))
```

Error rates should be zero or negligible. Compare against pre-migration baselines to confirm no regression.

### Phase 3: Switch to v2 storage

Once you're confident that v2 is working correctly and you no longer need to query data ingested before Phase 1, you can disable the v1 storage.

{{< admonition type="warning" >}}
After this step, data ingested before Phase 1 is no longer queryable through Pyroscope. The data still exists in object storage, but the v1 storage modules that serve it will be disabled. Make sure you don't need to query historical data from before the migration started.
{{< /admonition >}}

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true
```

#### Verify Phase 3

Verify that the Pyroscope pod has restarted:

```bash
kubectl get pods -n pyroscope -l app.kubernetes.io/instance=pyroscope
```

Verify that queries still return data:

```bash
kubectl port-forward -n pyroscope svc/pyroscope 4040:4040 &
PF_PID=$!
sleep 2
profilecli query series --url http://localhost:4040 --from "now-1h" --to "now"
kill $PF_PID
```

You should see series labels for recent profiling data. You can also open the Pyroscope UI at `http://localhost:4040` and verify that you can query recent profiles. An empty or errored UI indicates a problem — see [Rollback](#rollback).

## Microservices mode

If you deployed Pyroscope using the `values-micro-services.yaml` file as described in [Deploy on Kubernetes](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/deploy-kubernetes/helm/), follow the steps below.

### Phase 1: Deploy v2 components alongside v1

In this phase, you deploy the v2 components (segment-writer, metastore, compaction-worker, query-backend) alongside your existing v1 installation. The distributor starts writing to both storage backends simultaneously. The read path serves data from both v1 and v2.

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.microservices.enabled=true \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true
```

The `--reuse-values` flag preserves your existing configuration. Alternatively, you can pass your values file with `-f values.yaml`.

#### Verify Phase 1

After the upgrade completes, check that the new components are running:

```bash
kubectl get pods -n pyroscope -l app.kubernetes.io/instance=pyroscope
```

Check the Helm release notes for migration status:

```bash
helm get notes -n pyroscope pyroscope
```

You should see output similar to:

```
# Pyroscope v2 Migration is active

Write traffic will be written to:
- 100% v1: ingester
- 100% v2: segment-writer

Read traffic is served from v2 read path from as soon as data was first ingested to v2.
```

Also verify:

- The **metastore** raft cluster has elected a leader:

  ```bash
  kubectl logs -n pyroscope -l app.kubernetes.io/component=metastore --tail=500 | grep -i "entering leader state"
  ```

- The **segment-writer** ring is healthy. All instances should show as `ACTIVE`:

  ```bash
  kubectl port-forward -n pyroscope svc/pyroscope-distributor 4040:4040 &
  PF_PID=$!
  sleep 2
  curl -s http://localhost:4040/ring-segment-writer | grep -o 'ACTIVE' | wc -l
  kill $PF_PID
  ```

  The count should match the number of segment-writer instances.

### Phase 2: Validate v2 is working

Run both storage backends simultaneously for at least 24 hours before proceeding. During this time, you should be able to query data ingested to v2.

#### Verify data is being written to v2

Query recent profiling data. The v2 read path should serve data ingested after Phase 1. You can use `profilecli`, the Pyroscope UI, or the API to query profiles from the last hour and confirm results are returned:

```bash
kubectl port-forward -n pyroscope svc/pyroscope-query-frontend 4040:4040 &
PF_PID=$!
sleep 2
profilecli query series --url http://localhost:4040 --from "now-1h" --to "now"
kill $PF_PID
```

You should see series labels for the profiling data being ingested. If no results are returned, check the distributor and segment-writer logs for errors.

#### Verify v2 compaction is running

The compaction-worker compacts segments through the L0 &rarr; L1 &rarr; L2 levels. Verify that compaction jobs are completing:

```bash
kubectl logs -n pyroscope -l app.kubernetes.io/component=compaction-worker --tail=500 | grep "compaction finished successfully"
```

You should see log lines like:

```
msg="compaction finished successfully" input_blocks=20 output_blocks=1
```

Compaction typically starts within minutes of ingestion, the first block is created once enough segments accumulate for a shard.

#### Verify error rates are stable

Check that write and read error rates haven't increased since enabling v2. If you have Prometheus metrics configured, query error rates per component:

```promql
# Server-side errors by component (distributor, segment-writer, query-frontend, query-backend, etc.)
sum by (component) (rate(pyroscope_request_duration_seconds_count{status_code=~"5.."}[5m]))
```

All components should show zero or negligible error rates. Compare against pre-migration baselines to confirm no regression.

### Phase 3: Remove v1 components

Once you're confident that v2 is working correctly and you no longer need to query data ingested before Phase 1, you can remove the v1 components.

{{< admonition type="warning" >}}
After this step, data ingested before Phase 1 is no longer queryable through Pyroscope. The data still exists in object storage, but the v1 read path components (ingester, store-gateway, querier) that serve it will be removed. Make sure you don't need to query historical data from before the migration started.
{{< /admonition >}}

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=false \
  --set architecture.storage.v2=true
```

The Helm chart automatically removes v1-only components (ingester, compactor, store-gateway, querier, query-scheduler) when `architecture.storage.v1` is set to `false`, even if your values file or `--reuse-values` state still contains overrides for those components.

#### Verify Phase 3

Check that v1 components have been removed and v2 is serving all traffic:

```bash
# v1 components (ingester, store-gateway, querier, compactor, query-scheduler) should be gone
kubectl get pods -n pyroscope -l app.kubernetes.io/instance=pyroscope
```

Verify that queries still return data:

```bash
kubectl port-forward -n pyroscope svc/pyroscope-query-frontend 4040:4040 &
PF_PID=$!
sleep 2
profilecli query series --url http://localhost:4040 --from "now-1h" --to "now"
kill $PF_PID
```

You should see series labels for recent profiling data. You can also open the Pyroscope UI at `http://localhost:4040` and verify that you can query recent profiles. An empty or errored UI indicates a problem — see [Rollback](#rollback).

## Rollback

### During Phase 1 or Phase 2

Rolling back is straightforward — set `architecture.storage.v2=false` to remove the v2 components and return to v1-only:

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=false
```

Data written to v2 during the dual-ingest period is orphaned but doesn't affect v1 operation.

### During or after Phase 3

If you removed v1 components (Phase 3), rolling back requires redeploying them:

```bash
helm upgrade -n pyroscope pyroscope grafana/pyroscope \
  --reuse-values \
  --set architecture.storage.v1=true \
  --set architecture.storage.v2=true
```

This returns you to dual-ingest mode (Phase 1). Note that any data ingested between Phase 3 and the rollback was only written to v2 and won't be visible through the v1 read path.

## Helm values reference

The following Helm values control the v1/v2 storage configuration and migration behavior.

### Storage layer toggles

| Value                     | Type | Default | Description                                                                                         |
|---------------------------|------|---------|-----------------------------------------------------------------------------------------------------|
| `architecture.storage.v1` | bool | `true`  | Enable v1 storage and its components (ingester, store-gateway, querier, compactor).                 |
| `architecture.storage.v2` | bool | `false` | Enable v2 storage and its components (segment-writer, metastore, compaction-worker, query-backend). |

### Migration tuning

These values only apply when both `v1` and `v2` are enabled (dual-ingest mode). All values are under `architecture.storage.migration`.

| Value                          | Type   | Default  | Description                                                                                                                                                                                                                                                                 |
|--------------------------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `<prefix>.ingesterWeight`      | float  | `1.0`    | Fraction `[0, 1]` of write traffic sent to v1 ingesters.                                                                                                                                                                                                                    |
| `<prefix>.segmentWriterWeight` | float  | `1.0`    | Fraction `[0, 1]` of write traffic sent to v2 segment-writers.                                                                                                                                                                                                              |
| `<prefix>.queryBackend`        | bool   | `true`   | Enable the v2 query backend for reads.                                                                                                                                                                                                                                      |
| `<prefix>.queryBackendFrom`    | string | `"auto"` | RFC 3339 timestamp (e.g. `2025-01-01T00:00:00Z`) from which the v2 read path serves traffic. When set to `auto`, the query frontend consults the metastore per tenant to determine when v2 data first appeared. If no v2 data exists for a tenant, queries fall back to v1. |
