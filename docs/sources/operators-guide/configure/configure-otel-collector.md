---
description: Learn how to write profiles from OpenTelemetry Collector into Fire
menuTitle: Configure OTel Collector
title: Configure the OpenTelemetry Collector to write profiles into Fire
weight: 150
---

# Configure the OpenTelemetry Collector to write profiles into Fire

When using the [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/), you can write profiles into Fire via two options: `prometheusremotewrite` and `otlphttp`.

We recommend using the `prometheusremotewrite` exporter when possible because the remote write ingest path is tested and proven at scale.

## Remote Write

For the Remote Write, use the [`prometheusremotewrite`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/prometheusremotewriteexporter) exporter in the Collector:

In the `exporters` section add:

```yaml
exporters:
  prometheusremotewrite:
    endpoint: http://<fire-endpoint>/api/v1/push
```

And enable it in the `service.pipelines`:

```yaml
service:
  pipelines:
    profiles:
      receivers: [...]
      processors: [...]
      exporters: [..., prometheusremotewrite]
```

If you want to authenticate using basic auth, we recommend the [`basicauth`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/basicauthextension) extension:

```yaml
extensions:
  basicauth/prw:
    client_auth:
      username: username
      password: password

exporters:
  prometheusremotewrite:
    auth:
      authenticator: basicauth/prw
    endpoint: http://<fire-endpoint>/api/v1/push

service:
  extensions: [basicauth/prw]
  pipelines:
    profiles:
      receivers: [...]
      processors: [...]
      exporters: [..., prometheusremotewrite]
```

## OTLP

Fire supports native OTLP over HTTP. To configure the collector to use the OTLP interface, you use the [`otlphttp`](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/otlphttpexporter) exporter:

```yaml
exporters:
  otlphttp:
    endpoint: http://<fire-endpoint>/otlp
```

And enable it in `service.pipelines`:

```yaml
service:
  pipelines:
    profiles:
      receivers: [...]
      processors: [...]
      exporters: [..., otlphttp]
```

If you want to authenticate using basic auth, we recommend the [`basicauth`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/basicauthextension) extension:

```yaml
extensions:
  basicauth/otlp:
    client_auth:
      username: username
      password: password

exporters:
  otlphttp:
    auth:
      authenticator: basicauth/otlp
    endpoint: http://<fire-endpoint>/otlp

service:
  extensions: [basicauth/otlp]
  pipelines:
    profiles:
      receivers: [...]
      processors: [...]
      exporters: [..., otlphttp]
```
