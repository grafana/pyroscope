---
aliases:
  - /docs/mimir/latest/operators-guide/deploying-grafana-mimir/jsonnet/migrating-from-consul-to-memberlist/
description:
  Learn how to migrate from using Consul as KV store for hash rings to
  using memberlist without any downtime.
menuTitle: Migrating from Consul to memberlist
title: Migrating from Consul to memberlist KV store for hash rings without downtime
weight: 40
---

# Migrating from Consul to memberlist KV store for hash rings without downtime

Mimir Jsonnet uses memberlist as KV store for hash rings since Mimir 2.2.0.

Memberlist can be disabled by using the following configuration:

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: false
  }
}
```

If you are running Mimir hash rings with Consul and would like to migrate to memberlist without any downtime, you can follow instructions in this document.

## Step 1: Enable memberlist and multi KV store.

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
    multikv_migration_enabled: true,
  }
}
```

Step 1 configures components to use `multi` KV store, with `consul` as primary and memberlist as secondary stores.
This step requires rollout of all Mimir components.
After applying this step all Mimir components will expose [`/memberlist`]({{< relref "../../reference-http-api/index.md#memberlist-cluster" >}}) page on HTTP admin interface, which can be used to check health of memberlist cluster.

## Step 2: Enable KV store mirroring

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
    multikv_migration_enabled: true,
    multikv_mirror_enabled: true,  // Changed in this step.
  }
}
```

In this step we enable writes to primary KV store (Consul) to be mirrored into secondary store (memberlist).
Applying this change will not cause restart of Mimir components.

You can monitor following metrics to check if mirroring was enabled on all components and if it works correctly:

- `cortex_multikv_mirror_enabled` – shows which components have KV store mirroring enabled. All Mimir components should start mirroring to secondary KV store reloading runtime configuration.
- `rate(cortex_multikv_mirror_writes_total[1m])` – shows rate of writes to secondary KV store in writes per second.
- `rate(cortex_multikv_mirror_write_errors_total[1m])` – shows rate of write errors to secondary KV store, in errors per second.

After mirroring is enabled, you should see a key for each Mimir hash ring in the [Memberlist cluster information]({{< relref "../../reference-http-api/index.md#memberlist-cluster" >}}) admin page.
See [list of components that use hash ring]({{< relref "../../architecture/hash-ring/index.md" >}}).

## Step 3: Switch Primary and Secondary store

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
    multikv_migration_enabled: true,
    multikv_mirror_enabled: true,
    multikv_switch_primary_secondary: true,  // Changed in this step.
  }
}
```

This change will switch primary and secondary stores as used by `multi` KV.
From this point on Mimir components will use memberlist as primary KV store, and they will mirror updates to Consul.
This step does not require restart of Mimir components.

To see if all components started to use memberlist as primary store, please watch `cortex_multikv_primary_store` metric.

## Step 4: Disable mirroring to Consul

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
    multikv_migration_enabled: true,
    multikv_mirror_enabled: false,  // Changed in this step.
    multikv_switch_primary_secondary: true,
  }
}
```

This step does not require restart of any Mimir component. After applying the change components will stop writing ring updates to Consul, and will only use memberlist.
You can watch `cortex_multikv_mirror_enabled` metric to see if all components have picked up updated configuration.

## Step 5: Disable `multi` KV Store

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
    multikv_migration_enabled: false,  // Changed in this step.
    multikv_mirror_enabled: false,
    multikv_switch_primary_secondary: true,
    multikv_migration_teardown: true,  // Added in this step.
  }
}
```

This configuration change will cause a new rollout of all components.
After the restart components will no longer use `multi` KV store and will be configured to use memberlist only.
We use `multikv_migration_teardown` to preserve runtime configuration for `multi` KV store for components that haven't restarted yet.

All `cortex_multikv_*` metrics are only exposed by components that use `multi` KV store. As components restart, these metrics will disappear.

> **Note**: setting `multikv_migration_enabled: false` while keeping `memberlist_ring_enabled: true` will also remove Consul! That's expected, since Consul is not used anymore – mirroring to it was disabled in step 4.

If you need to keep consul running, you can explicitly set `consul_enabled: true` in `_config`.

## Step 6: Cleanup

We have successfully migrated Mimir cluster from using Consul to memberlist without any downtime!
As a final step, we can remove all migration-related config options:

- `multikv_migration_enabled`
- `multikv_mirror_enabled`
- `multikv_switch_primary_secondary`
- `multikv_migration_teardown`

Our final memberlist configuration will be:

```jsonnet
{
  _config+:: {
    memberlist_ring_enabled: true,
  }
}
```

This will not trigger new restart of Mimir components. After applying this change, you are finished.
