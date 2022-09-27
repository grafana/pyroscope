---
aliases:
  - /docs/mimir/latest/operators-guide/securing/authentication-and-authorization/
description: Learn how to configure and run Grafana Mimir with multi-tenancy.
menuTitle: Authentication and authorization
title: Grafana Mimir authentication and authorization
weight: 20
---

# Grafana Mimir authentication and authorization

Grafana Mimir is a multi-tenant system where tenants can query metrics and alerts that include their tenant ID.
The query takes the tenant ID from the `X-Scope-OrgID` parameter that exists in the HTTP header of each request, for example `X-Scope-OrgID: <TENANT-ID>`.
You can federate queries across multiple tenants by using `true` in `-tenant-federation.enabled=true`. When you specify tenant IDs, separate them with a pipe (`|`) character in the 'X-Scope-OrgID' header, as in the example `X-Scope-OrgID: tenant-1|tenant-2|tenant-3`.

To protect Grafana Mimir from accidental or malicious calls, you must add a layer of protection such as a reverse proxy that authenticates requests and injects the appropriate tenant ID into the `X-Scope-OrgID` header.

## Configuring Prometheus remote write

For more information about Prometheus remote write configuration, refer to [remote write](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write).

## With an authenticating reverse proxy

To use bearer authentication with a token stored in a file, the remote write configuration block includes the following parameters:

```yaml
authorization:
  type: Bearer
  credentials_file: <PATH TO BEARER TOKEN FILE>
```

To use basic authentication with a username and password stored in a file, the remote write configuration block includes the following parameters:

```yaml
basic_auth:
  username: <AUTHENTICATION PROXY USERNAME>
  password_file: <PATH TO AUTHENTICATION PROXY PASSWORD FILE>
```

## Without an authenticating reverse proxy

To configure the `X-Scope-OrgID` header directly, the remote write configuration block includes the following parameters:

```yaml
headers:
  "X-Scope-OrgID": <TENANT ID>
```

## Extracting tenant ID from Prometheus labels

In trusted environments where you want to split series on Prometheus labels, you can run [cortex-tenant](https://github.com/blind-oracle/cortex-tenant) between a Prometheus server and Grafana Mimir.

> **Note:** cortex-tenant is a third-party community project that is not maintained by Grafana Labs.

When proxying the timeseries to Grafana Mimir, you can configure cortex-tenant to use specified labels as the `X-Scope-OrgID` header.

To configure cortex-tenant, refer to [configuration](https://github.com/blind-oracle/cortex-tenant#configuration).

## Disabling multi-tenancy

To disable multi-tenant functionality, pass the following argument to every Grafana Mimir component:

`-auth.multitenancy-enabled=false`

After you disable multi-tenancy, Grafana Mimir components internally set the tenant ID to the string `anonymous` for every request.

To set an alternative tenant ID, use the `-auth.no-auth-tenant` flag.

> **Note**: Not all tenant IDs are valid. For more information about tenant ID restrictions, refer to [About tenant IDs]({{< relref "../configure/about-tenant-ids.md" >}}).
