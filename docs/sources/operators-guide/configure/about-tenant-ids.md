---
aliases:
  - /docs/fire/latest/operators-guide/configuring/about-tenant-ids/
description: Learn about tenant ID restrictions.
menuTitle: About tenant IDs
title: About Grafana Fire tenant IDs
weight: 40
---

# About Grafana Fire tenant IDs

Within a Grafana Fire cluster, the tenant ID is the unique identifier of a tenant.
For information about how Grafana Fire components use tenant IDs, refer to [Authentication and authorization]({{< relref "../secure/authentication-and-authorization.md" >}}).

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
> **Note:** The tenant ID `__fire_cluster` is unsupported because its name is used internally by Fire.

All other characters, including slashes and whitespace, are not supported.
