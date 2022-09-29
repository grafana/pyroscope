---
title: "Configure TSDB block upload"
menuTitle: "Configure TSDB block upload"
description: "Learn how to configure Grafana Fire to enable TSDB block upload"
weight: 120
---

# Configure TSDB block upload

Grafana Fire supports uploading of historic TSDB blocks, sourced from Prometheus, Cortex, or even other
Grafana Fire installations. Upload from Thanos is currently not supported; for more information, see [Known limitations of TSDB block upload]({{< relref "#known-limitations-of-tsdb-block-upload" >}}).

To make performing block upload simple, we've built support for it into Fire's CLI tool, [firetool]({{< relref "../tools/firetool.md" >}})). See the [firetool backfill]({{< relref "../tools/firetool.md#backfill" >}}) documentation to learn more.

Block upload is still considered experimental and is therefore disabled by default. You can enable it via the `-compactor.block-upload-enabled`
CLI flag, or via the corresponding `limits.compactor_block_upload_enabled` configuration parameter:

```yaml
limits:
  # Enable TSDB block upload
  compactor_block_upload_enabled: true
```

## Enable TSDB block upload per tenant

If your Grafana Fire has multi-tenancy enabled, you can still use the preceding method to enable
TSDB block upload for all tenants. If instead you wish to enable it per tenant, you can use the
runtime configuration to set a per-tenant override:

1. Enable [runtime configuration]({{< relref "about-runtime-configuration.md" >}}).
1. Add an override for the tenant that should have TSDB block upload enabled:

```yaml
overrides:
  tenant1:
    compactor_block_upload_enabled: true
```

## Known limitations of TSDB block upload

### Thanos blocks cannot be uploaded

Because Thanos blocks contain unsupported labels among their metadata, they cannot be uploaded.

For information about limitations that relate to importing blocks from Thanos as well as existing workarounds, see
[Migrating from Thanos or Prometheus to Grafana Fire]({{< relref "../../migration-guide/migrating-from-thanos-or-prometheus.md" >}}).

### No validation on imported blocks

Grafana Fire does not validate that the uploaded blocks are well-formed. This means that users could upload malformed blocks to Grafana Fire. These malformed blocks could potentially cause problems on the Fire query path or for the operation of Fire's compactor component.

We intend to add validation of uploaded blocks in a future release, which would allow us to identify and reject malformed blocks at upload time and prevent any downstream impact to Grafana Fire.

### The results-cache needs flushing

After uploading one or more blocks, the results-cache needs flushing. The reason is that Grafana Fire caches query results
for queries that don’t touch the most recent 10 minutes of data. After uploading blocks however, queries may return different
results (because new data was uploaded). Therefore cached results may be wrong, meaning the cache should manually be flushed
after uploading blocks.

### Blocks that are too new will not be queryable until later

When queriers receive a query for a given [start, end] period, they consult this period to decide whether to read
data from storage, ingesters, or both. Say `-querier.query-store-after` is set to `12h`. It means that a query
`[now-13h, now]` will read data from storage. But a query `[now-5h, now]` will _not_. If a user uploads blocks that are
“too new”, the querier may not query them, because it is configured to only query ingesters for “fresh” time ranges.
