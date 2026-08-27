---
title: "Link tracing and profiling with Span Profiles"
menuTitle: "Link traces and profiles"
description: "Learn how to configure the client to Link tracing and profiling with span profiles."
weight: 400
---

# Link tracing and profiling with Span Profiles

Span Profiles link tracing and profiling data, so that you can look at the profile of a single request or trace span instead of an aggregate of everything a service did.
This makes it possible to move from a high-level trace view to the code that made one particular span slow.

## How span profiles work

A span profile is not a separate kind of profile.
Span-aware instrumentation records which trace span is active as the profiler takes each sample, and tags that sample with the span's identity.
Pyroscope recognizes two labels for this:

- `span_id`: the ID of the span the sample was taken during. Some integrations send this as `profile_id`, which Pyroscope accepts as a legacy alias.
- `trace_id`: the ID of the trace that span belongs to.

Integrations may attach the span name as well, as an ordinary label.

Because the labels are attached per sample, a single profile contains samples belonging to many different spans.
Querying by span ID returns the samples of that span rather than a whole profile, which is what makes it possible to profile one request.

By default, the integrations label only the local root span, meaning the first span created inside the process.
Samples collected while its child spans run are included in the root span's profile.
The Go and Java integrations can be configured to label every span instead.

Spans that can have a profile are marked with the `pyroscope.profile.id` span attribute, whose value is the span ID despite the name.
Grafana uses that attribute to offer a link from a span to its profile.

Key benefits and features:

- Deep analysis: Understand the specifics of code execution within particular time frames, offering granular insights into application performance
- Seamless integration: Smoothly transition from a high-level trace overview to detailed profiling of specific trace spans within Grafana’s trace view
- Efficiency and cost savings: Quickly identify and address performance issues, reducing troubleshooting time and operational costs

## What to expect

- The `pyroscope.profile.id` attribute marks spans that *can* have a profile. It does not guarantee that any samples were collected for that span.
- A span shorter than the profiler's sample interval may produce no samples at all. The CPU profiler collects stack traces 100 times per second by default, so spans shorter than roughly 10ms often have nothing to show.
- The profile types available for span profiles depend on the language and the profiler it uses. Refer to the page for your language.
- If your instrumentation records span names, avoid dynamic names that embed per-request identifiers such as URLs or SQL queries, because those can significantly degrade performance.

{{< admonition type="note">}}
Span profiling is only effective on spans longer than 20ms to ensure statistical accuracy.
{{< /admonition >}}

## Get started

Select an option from the list below:

- Configure Pyroscope: Begin sending profiling data to unlock the full potential of Span Profiles
- Client-side packages: Easily link traces and profiles using available packages for Go, Java, Ruby, .NET, and Python
  - Go: [Span profiles with Traces to profiles (Go)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/go-span-profiles/)
  - Java: [Span profiles with Traces to profiles (Java)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/java-span-profiles/)
  - Ruby: [Span profiles with Traces to profiles (Ruby)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/ruby-span-profiles/)
  - .NET: [Span profiles with Traces to profiles (.NET)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/dotnet-span-profiles/)
  - Python: [Span profiles with Traces to profiles (Python)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/python-span-profiles/)
- [Configure the Tempo data source in Grafana or Grafana Cloud](/docs/grafana-cloud/connect-externally-hosted/data-sources/tempo/configure-tempo-data-source/) to discover linked traces and profiles.

To learn more, check out the product announcement blog: [Introducing Span Profiles](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/).
