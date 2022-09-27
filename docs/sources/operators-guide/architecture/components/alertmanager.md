---
title: "(Optional) Grafana Mimir Alertmanager"
menuTitle: "(Optional) Alertmanager"
description: "The Alertmanager groups alert notifications and routes them to various notification channels."
weight: 100
---

# (Optional) Grafana Mimir Alertmanager

The Mimir Alertmanager adds multi-tenancy support and horizontal scalability to the [Prometheus Alertmanager](https://prometheus.io/docs/alerting/alertmanager/).
The Mimir Alertmanager is an optional component that accepts alert notifications from the [Mimir ruler]({{< relref "ruler/index.md" >}}).
The Alertmanager deduplicates and groups alert notifications, and routes them to a notification channel, such as email, PagerDuty, or OpsGenie.

## Multi-tenancy

Like other Mimir components, multi-tenancy in the Mimir Alertmanager uses the tenant ID header.
Each tenant has an isolated alert routing configuration and Alertmanager UI.

### Tenant configurations

Each tenant has an Alertmanager configuration that defines notifications receivers and alerting routes.
The Mimir Alertmanager uses the same configuration file used by the Prometheus Alertmanager.

> **Note:** The Mimir Alertmanager exposes the configuration API according to the path set by the `-server.path-prefix` flag. It does not use the path set by the `-http.alertmanager-http-prefix` flag.
> With the default configuration of `-server.path-prefix`, the Alertmanager URL used as the `mimirtool` `--address` flag has no path portion.

The following sample command shows how to upload a tenant's Alertmanager configuration using `mimirtool`:

```bash
mimirtool alertmanager load <ALERTMANAGER CONFIGURATION FILE>  \
  --address=<ALERTMANAGER URL>
  --id=<TENANT ID>
```

The following sample command shows how to retrieve a tenant's Alertmanager configuration using `mimirtool`:

```bash
mimirtool alertmanager get \
  --address=<ALERTMANAGER URL>
  --id=<TENANT ID>
```

The following sample commands shows how to delete a tenant's Alertmanager configuration using `mimirtool`:

```bash
mimirtool alertmanager delete \
  --address=<ALERTMANAGER URL>
  --id=<TENANT ID>
```

After the tenant uploads an Alertmanager configuration, the tenant can access the Alertmanager UI at the `/alertmanager` endpoint.

#### Fallback configuration

When a tenant doesn't have a Alertmanager configuration, the Grafana Mimir Alertmanager uses a fallback configuration, if configured.
By default, there is no fallback configuration set.
Specify a fallback configuration using the `-alertmanager.configs.fallback` command-line flag.

> **Warning**: Without a fallback configuration or a tenant specific configuration, the Alertmanager UI is inaccessible and ruler notifications for that tenant fail.

### Tenant limits

The Grafana Mimir Alertmanager has a number of per-tenant limits documented in [`limits`]({{< relref "../../configure/reference-configuration-parameters/index.md#limits" >}}).
Each Mimir Alertmanager limit configuration parameter has an `alertmanager` prefix.

## Alertmanager UI

The Mimir Alertmanager exposes the same web UI as the Prometheus Alertmanager at the `/alertmanager` endpoint.

When running Grafana Mimir with multi-tenancy enabled, the Alertmanager requires that any HTTP request include the tenant ID header.
Tenants only see alerts sent to their Alertmanager.

For a complete reference of the tenant ID header and Alertmanager endpoints, refer to [HTTP API]({{< relref "../../reference-http-api/index.md" >}}).

You can configure the HTTP path prefix for the UI and the HTTP API:

- `-http.alertmanager-http-prefix` configures the path prefix for Alertmanager endpoints.
- `-alertmanager.web.external-url` configures the source URLs generated in Alertmanager alerts and from where to fetch web assets.

> **Note:** Unless you are using a reverse proxy in front of the Alertmanager API that rewrites routes, the path prefix set in `-alertmanager.web.external-url` must match the path prefix set in `-http.alertmanager-http-prefix` (`/alertmanager` by default).
> If the path prefixes do not match, HTTP requests routing might not work as expected.

### Using a reverse proxy

When using a reverse proxy, use the following settings when you configure the HTTP path:

- Set `-http.alertmanager-http-prefix` to match the proxy path in your reverse proxy configuration.
- Set `-alertmanager.web.external-url` to the URL served by your reverse proxy.

## Sharding and replication

The Alertmanager shards and replicates alerts by tenant.
Sharding requires that the number of Alertmanager replicas is greater-than or equal-to the replication factor configured by the `-alertmanager.sharding-ring.replication-factor` flag.

Grafana Mimir Alertmanager replicas use [hash ring]({{< relref "../hash-ring/index.md" >}}) that is stored in the KV store to discover their peers.
This means that any Mimir Alertmanager replica can respond to any API or UI request for any tenant.
If the Mimir Alertmanager replica receiving the HTTP request doesn't own the tenant to which the request belongs, the request is internally routed to the appropriate replica.

To configure the Alertmanagers' hash ring, refer to [configuring hash rings]({{< relref "../../configure/configuring-hash-rings.md" >}}).

> **Note:** When running with a single tenant, scaling the number of replicas to be greater than the replication factor offers no benefits as the Mimir Alertmanager shards by tenant and not individual alerts.

### State

The Mimir Alertmanager stores the alerts state on local disk at the location configured using `-alertmanager.storage.path`.

> **Warning:**
> When running the Mimir Alertmanager without replication, ensure persistence of the `-alertmanager.storage.path` directory to avoid losing alert state.

The Mimir Alertmanager also periodically stores the alert state in the storage backend configured with `-alertmanager-storage.backend`.
When an Alertmanager starts, it attempts to load the alerts state for a given tenant from other Alertmanager replicas. If the load from other Alertmanager replicas fails, the Alertmanager falls back to the state that is periodically stored in the storage backend.

In the event of a cluster outage, this fallback mechanism recovers the backup of the previous state. Because backups are taken periodically, this fallback mechanism does not guarantee that the lastest state is restored.
