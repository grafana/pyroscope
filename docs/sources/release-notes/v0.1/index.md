---
title: "Pyroscope version 0.1 release notes"
menuTitle: "V0.1 release notes"
description: "Release notes for Pyroscope version 0.1"
weight: 10
---

# Pyroscope version 0.1 release notes

![Pyroscope Logo](phlare-logo.png)


The Pyroscope team is excited to announce the first release. We’re committed to building the best continuous profiling experience in Grafana and contributing to that space along with the open-source community.

From the first release you can expect:

- **Pyroscope is a horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system** with similar architecture to Grafana Mimir, Grafana Loki, and Grafana Tempo.
- **Easy to get started with guides** covering Helm, Tanka, and docker-compose installations.
- **A fully integrated data source in Grafana** to correlate your continuous profiling data with other observability signals using Grafana Explore and dashboards. The native flame graph panel visualization can also be used by other profiling data sources.
- **Pyroscope packages an Agent** for pulling profiles directly from your applications like Prometheus. We have also provided detailed documentation about how to profile your application written in **Go, Java/JVM, Python, and Rust**.

![Pyroscope UI](phlare-ui.png)

Ready to give it a try, follow our [getting started]({{< relref "../../get-started/" >}}) documentation.

> **Note:** This is an early release and there are a couple of limitations:
>
> - As we iterate on Pyroscope, the APIs are still subject to change and we can’t yet provide stability. This is most likely going to be guaranteed in the future 1.0 release.
> - While we can archive your data to long-term storage, we currently do not support querying it back.

We are keen to hear your feedback and ideas on what we should focus on next. Get in touch with the team using:

- [Slack](https://grafana.slack.com/archives/C047CCW6YM8)
- [Github Discussions](https://github.com/grafana/pyroscope/discussions)
