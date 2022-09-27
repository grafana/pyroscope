---
title: "Grafana Mimir tenant injector"
menuTitle: "Tenant injector"
description: "Use the tenant injector to query data for a tenant during development and troubleshooting."
weight: 20
---

# Grafana Mimir tenant injector

The tenant injector is a standalone HTTP proxy that injects the `X-Scope-OrgID` header with a value, which you specify via the `-tenant-id` flag into incoming HTTP requests, and then forwards the modified requests to the URL you specify via the `-remote-address` flag.

You can use the tenant injector to query data for a tenant during development or troubleshooting.

```
Usage of tenant-injector:
  -local-address string
    	Local address to listen on (host:port or :port). (default ":8080")
  -remote-address string
    	URL of target to forward requests to to (eg. http://domain.com:80).
  -tenant-id string
    	Tenant ID to inject to proxied requests.
```
