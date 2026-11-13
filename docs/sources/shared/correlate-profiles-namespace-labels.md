---
headless: true
description: Shared file for namespace label profile correlation guidance.
---

[//]: # 'Shared content explaining how namespace labels correlate profiles with trace spans and Application Observability.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/dotnet.md'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/java.md'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/go_push.md'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/python.md'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/ruby.md'
[//]: # '/pyroscope/docs/sources/configure-client/language-sdks/nodejs.md'
[//]: #
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'

By default, the Tempo data source maps the span's `service.namespace` attribute to the `service_namespace` profile label when you use [Traces to profiles](https://grafana.com/docs/pyroscope/latest/view-and-analyze-profile-data/traces-to-profiles/) to jump from a trace span to its profiling data.
If your profiles don't carry that label, the embedded flame graph in the trace view stays empty.

Pyroscope derives the `service_name` label from the application name you configure, which identifies the service.
The `service_namespace` label adds the namespace that the trace view matches on.
The `service_namespace` label follows the OpenTelemetry `service.namespace` resource attribute convention.

If the span attribute and your profile label don't match, you can remap them instead of adding a label.
In the Tempo data source, edit the **Traces to profiles** tag mapping so the span attribute maps to the label your profiles use.
Refer to [Configure the Tempo data source](https://grafana.com/docs/grafana/latest/datasources/tempo/configure-tempo-data-source/).

Traces to profiles works with Pyroscope, Grafana Enterprise, and Grafana Cloud.

### Show profiles in Application Observability

The Grafana Cloud [Application Observability](https://grafana.com/docs/grafana-cloud/monitor-applications/application-observability/) **Profiles** tab filters the flame graph on the `service_name` and `namespace` labels.
It uses `namespace`, not `service_namespace`.
To show your profiles there as well, add a `namespace` label with the same value.

Alternatively, keep a single namespace label and map the entity's namespace property to it when you create a profiles correlation configuration.
Refer to [Configure profiles correlation](https://grafana.com/docs/grafana-cloud/platform/knowledge-graph/configure/telemetry-correlation/profiles/).
