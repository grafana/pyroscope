---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/about-ip-address-logging/
description: Troubleshoot errors by logging IP addresses of reverse proxies.
menuTitle: About IP address logging of a reverse proxy
title: About Grafana Phlare IP address logging of a reverse proxy
weight: 60
---

# About Grafana Phlare IP address logging of a reverse proxy

If a reverse proxy is used in front of Phlare, it might be difficult to troubleshoot errors.
You can use the following settings to log the IP address passed along by the reverse proxy in headers such as `X-Forwarded-For`.

- `-server.log-source-ips-enabled`

  Set this to `true` to add IP address logging when a `Forwarded`, `X-Real-IP`, or `X-Forwarded-For` header is used. A field called `sourceIPs` is added to error logs when data is pushed into Grafana Phlare.

- `-server.log-source-ips-header`

  The header field stores the source IP addresses and is used only if `-server.log-source-ips-enabled` is `true`, and if `-server.log-source-ips-regex` is set. If you do not set these flags, the default `Forwarded`, `X-Real-IP`, or `X-Forwarded-For` headers are searched.

- `-server.log-source-ips-regex`

  A regular expression that is used to match the source IPs. The regular expression must contain at least one capturing group, the first of which is returned. This flag is used only if `-server.log-source-ips-enabled` is `true` and if `-server.log-source-ips-header` is set.
