---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/about-tenant-ids/
  - /docs/phlare/latest/configure-server/about-tenant-ids/
description: Learn about tenant ID restrictions.
menuTitle: About tenant IDs
title: About Grafana Pyroscope tenant IDs
weight: 40
---

# About Grafana Pyroscope tenant IDs

Grafana Pyroscope is a multi-tenant system where tenants can query profiles that include their tenant ID.
Within a Grafana Pyroscope cluster, the tenant ID is the unique identifier of a tenant.
The query takes the tenant ID from the `X-Scope-OrgID` parameter that exists in the HTTP header of each request, for example `X-Scope-OrgID: <TENANT-ID>`.

To push profiles to Grafana Pyroscope for a specific tenant, refer to [Configure the Agent]({{< relref "../configure-client" >}}).

> By default multi-tenancy is disabled, the tenant ID is ignored and all profiles are stored and retrieved with the same tenant (`anonymous`).
>
>To enable multi-tenancy, add the `multitenancy_enabled` parameter to the Grafana Pyroscope configuration file and set it to `true`. Alternatively you can also use command line arguments to enable multi-tenancy, for example `--auth.multitenancy-enabled=true`.

## Restrictions

Tenant IDs cannot be longer than 150 bytes or characters in length and can only include the following supported characters:

- Alphanumeric characters
  - `0-9`
  - `a-z`
  - `A-Z`
- Special characters
  - Exclamation point (`!`)
  - Hyphen (`-`)
  - Underscore (`_`)
  - Single period (`.`)
  - Asterisk (`*`)
  - Single quote (`'`)
  - Open parenthesis (`(`)
  - Close parenthesis (`)`)

> **Note:** For security reasons, `.` and `..` are not valid tenant IDs.

All other characters, including slashes and whitespace, are not supported.
