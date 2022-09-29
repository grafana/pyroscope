---
aliases:
  - ../visualizing-profiles/deploying-monitor-mixin/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/deploying-monitoring-mixin/
description: Learn how to deploy the Grafana Fire monitoring mixin.
menuTitle: Deploying the monitoring mixin
title: Deploying the Grafana Fire monitoring mixin
weight: 20
---

# Deploying the Grafana Fire monitoring mixin

Grafana Fire exposes a `/profiles` endpoint returning Prometheus profiles. You can configure your Prometheus server to scrape Grafana Fire or you can use the built-in functionality of the [Helm chart to automatically send these profiles to a remote]({{< relref "collecting-profiles-and-logs.md" >}}).
The endpoint is exposed on the Fire HTTP server address / port which can be customized through `-server.http-listen-address` and `-server.http-listen-port` CLI flags or their respective YAML [config options]({{< relref "../configure/reference-configuration-parameters/index.md" >}}).

## Dashboards and alerts

Grafana Fire is shipped with a comprehensive set of production-ready Grafana dashboards and alerts to monitor the state and health of a Fire cluster.

Dashboards provide both a high-level and in-depth view of every aspect of a Grafana Fire cluster.
You can take a look at all the available dashboards in [this overview]({{< relref "dashboards/_index.md" >}}).

Alerts allow you to monitor the health of a Fire cluster. For each alert, we provide detailed [runbooks]({{< relref "../fire-runbooks/_index.md" >}}) to further investigate and fix the issue.

The [requirements documentation]({{< relref "requirements.md" >}}) lists prerequisites for using the Grafana Fire dashboards and alerts.

The [installation instructions]({{< relref "installing-dashboards-and-alerts.md" >}}) show available options to install Grafana Fire dashboards and alerts.
