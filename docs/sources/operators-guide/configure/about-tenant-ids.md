---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/about-tenant-ids/
description: Learn about tenant ID restrictions.
menuTitle: About tenant IDs
title: About Grafana Phlare tenant IDs
weight: 40
---

# About Grafana Phlare tenant IDs

Grafana Phlare is a multi-tenant system where tenants can query profiles that include their tenant ID.
Within a Grafana Phlare cluster, the tenant ID is the unique identifier of a tenant.
The query takes the tenant ID from the `X-Scope-OrgID` parameter that exists in the HTTP header of each request, for example `X-Scope-OrgID: <TENANT-ID>`.

To push profiles to Grafana Phlare for a specific tenant refer to [Configure the Agent]({{<relref "../configure-agent/_index.md">}}).

## Restrictions

Tenant IDs must be less-than or equal-to 150 bytes or characters in length and can only include the following supported characters:

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
