---
aliases:
  - /docs/pyroscope/latest/configure-server/authentication/
description: Authenticate and harden access to Grafana Pyroscope with multitenancy and reverse proxies.
keywords:
  - authentication
  - authorization
  - multitenancy
  - security
  - nginx
menuTitle: Authentication
title: Authentication and access control
weight: 150
---

# Authentication and access control

Grafana Pyroscope does **not** ship with built-in user accounts, API keys, or a login system for the UI and HTTP APIs.
After the removal of the legacy self-hosted user store, production deployments are expected to:

1. Put Pyroscope behind a reverse proxy or API gateway that performs authentication (and optionally TLS termination).
2. Use **multitenancy** (`X-Scope-OrgID`) when more than one logical tenant must share a cluster.
3. Configure agents and SDKs to send credentials that the proxy understands (typically HTTP basic auth or a bearer token).

This page describes the options Pyroscope provides and gives example proxy setups for single-tenant and multi-tenant deployments.

## What Pyroscope authenticates

| Mechanism | Enabled by | Behavior |
| --------- | ---------- | -------- |
| Tenant ID header | `multitenancy_enabled: true` / `-auth.multitenancy-enabled=true` | Requires `X-Scope-OrgID` on every request. Isolates profile data per tenant. Does **not** verify passwords or tokens by itself. |
| Anonymous tenant | Multitenancy disabled (default) | All data is stored under the tenant ID `anonymous`. The `X-Scope-OrgID` header is ignored. |
| HTTP basic auth / bearer tokens | Not implemented in the server | Configure these on a reverse proxy, service mesh, or API gateway in front of Pyroscope. Clients such as Grafana Alloy already support `basic_auth` when calling the server. |

The UI and admin-style HTTP endpoints are reachable by anyone who can open the server port unless you protect them at the network or proxy layer.
Treat an unauthenticated Pyroscope listener as an internal-only endpoint.

## Multitenancy

Multitenancy is the server-side switch that makes tenant identity mandatory.

### Enable multitenancy

In the configuration file:

```yaml
multitenancy_enabled: true
```

Or on the command line:

```bash
pyroscope -auth.multitenancy-enabled=true
```

When multitenancy is enabled:

* Every write and read request must include the `X-Scope-OrgID` HTTP header.
* Missing or empty org IDs are rejected with HTTP `401 Unauthorized`.
* Profile data for different tenants is stored and queried separately.

When multitenancy is disabled (default):

* Pyroscope injects the tenant ID `anonymous` for all requests.
* `X-Scope-OrgID` is not required and is not used for isolation.

Tenant ID format rules are documented in [Tenant IDs](../about-tenant-ids/).

### Clients and the tenant header

* **Grafana Alloy / agents**: set the tenant ID in the Pyroscope export block (for example `tenant_id` / headers depending on component version) so `X-Scope-OrgID` is attached automatically.
* **Language SDKs**: configure the tenant header or use a proxy that adds it.
* **Grafana datasource**: set the tenant ID in the Pyroscope data source so Explore queries the correct tenant.
* **curl**:

```bash
curl -H 'X-Scope-OrgID: team-a' http://pyroscope.internal:4040/ready
```

## Recommended pattern: authenticate at a reverse proxy

Because Pyroscope does not validate end-user credentials, put an authenticating proxy in front of it and only expose the proxy.

```text
SDK / Alloy / Grafana UI  -->  reverse proxy (authn + TLS)  -->  Pyroscope
                                      |
                                      +-- optional: inject X-Scope-OrgID from the authenticated user
```

Benefits:

* One place to rotate credentials or integrate SSO (OAuth2, OIDC, LDAP).
* Network policy can block direct access to the Pyroscope pod/port.
* Works for both the pull path (UI/Grafana) and the push path (agents/SDKs).

### Single-tenant example (nginx basic auth)

Use this when everyone shares one tenant (multitenancy left disabled) and you only need to keep strangers out.

```nginx
# /etc/nginx/conf.d/pyroscope.conf
upstream pyroscope {
    server pyroscope:4040;
}

server {
    listen 443 ssl;
    server_name pyroscope.example.com;

    ssl_certificate     /etc/nginx/tls/fullchain.pem;
    ssl_certificate_key /etc/nginx/tls/privkey.pem;

    auth_basic           "Pyroscope";
    auth_basic_user_file /etc/nginx/htpasswd/pyroscope;

    location / {
        proxy_pass         http://pyroscope;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;

        # Large profile payloads
        client_max_body_size 32m;
    }
}
```

Create the password file with `htpasswd` and mount it into nginx.
Point Grafana Alloy or SDKs at `https://pyroscope.example.com` with matching basic auth:

```alloy
pyroscope.write "default" {
  endpoint {
    url = "https://pyroscope.example.com"
    basic_auth {
      username = "alloy"
      password = env("PYROSCOPE_PASSWORD")
    }
  }
}
```

Language SDKs typically accept `PYROSCOPE_BASIC_AUTH_USER` / `PYROSCOPE_BASIC_AUTH_PASSWORD` or equivalent configuration fields. See the [client configuration](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/) guides for each language.

### Multi-tenant example (nginx maps user → tenant)

Enable multitenancy on Pyroscope and derive `X-Scope-OrgID` from the authenticated user so clients cannot pick an arbitrary tenant.

```yaml
# pyroscope config
multitenancy_enabled: true
```

```nginx
upstream pyroscope {
    server pyroscope:4040;
}

# Map basic-auth user to tenant ID
map $remote_user $pyroscope_tenant {
    default       "";
    "team-a-user" "team-a";
    "team-b-user" "team-b";
}

server {
    listen 443 ssl;
    server_name pyroscope.example.com;

    ssl_certificate     /etc/nginx/tls/fullchain.pem;
    ssl_certificate_key /etc/nginx/tls/privkey.pem;

    auth_basic           "Pyroscope";
    auth_basic_user_file /etc/nginx/htpasswd/pyroscope;

    location / {
        # Reject users that are not in the map
        if ($pyroscope_tenant = "") {
            return 403;
        }

        proxy_pass         http://pyroscope;
        proxy_http_version 1.1;
        proxy_set_header   Host $host;
        # Inject tenant from the authenticated user; clients cannot override it here
        proxy_set_header   X-Scope-OrgID $pyroscope_tenant;

        client_max_body_size 32m;
    }
}
```

Each agent then authenticates as its own user (or you run one gateway per tenant). Grafana data sources should use the same proxy URL and credentials for the tenant they are allowed to view.

### OAuth2 / OIDC example (oauth2-proxy)

For browser SSO in front of the Pyroscope UI (and optionally the API):

```text
Browser --> oauth2-proxy (OIDC with your IdP) --> Pyroscope
```

Run [oauth2-proxy](https://github.com/oauth2-proxy/oauth2-proxy) (or an equivalent gateway) with:

* Your identity provider client ID/secret
* `upstream` set to the Pyroscope service
* Headers forwarded to Pyroscope

For multitenancy, configure the proxy to set `X-Scope-OrgID` from a claim (for example `org_id` or `groups`) rather than trusting a client-supplied header. Exact claim-to-header configuration depends on the proxy; the important part is that **only the proxy** can set the tenant header on the internal network.

## Grafana

When Grafana reaches Pyroscope:

1. Create a Pyroscope data source pointing at the **proxy** URL, not the raw pod IP.
2. Configure the data source HTTP credentials (basic auth or forward OAuth) to match the proxy.
3. If multitenancy is enabled, set the tenant ID on the data source so queries send `X-Scope-OrgID`.

Direct browser access to the Pyroscope UI should also go through the same authenticated entrypoint if the UI is enabled in your deployment.

## Hardening checklist

* Bind Pyroscope to an internal listen address and block public access with NetworkPolicies / security groups.
* Terminate TLS at the proxy or mesh; do not send basic auth over plain HTTP outside a trusted network.
* Prefer short-lived tokens (OIDC) over long-lived shared passwords where possible.
* With multitenancy enabled, ensure external clients **cannot** reach Pyroscope without the proxy, otherwise they can send any `X-Scope-OrgID`.
* Restrict which tenants a credential can access (per-user mapping, per-tenant gateways, or mesh authz).
* Keep backend object storage credentials only on the Pyroscope servers; never embed them in agents.

## Related topics

* [Tenant IDs](../about-tenant-ids/)
* [About configurations](../about-configurations/)
* [Configure the client](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/)
* [Reference: configuration parameters](../reference-configuration-parameters/)
