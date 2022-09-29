---
aliases:
  - ../visualizing-profiles/installing-dashboards-and-alerts/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/installing-dashboards-and-alerts/
description: Learn how to install Grafana Fire dashboards and alerts.
menuTitle: Installing dashboards and alerts
title: Installing Grafana Fire dashboards and alerts
weight: 30
---

# Installing Grafana Fire dashboards and alerts

Grafana Fire is shipped with a comprehensive set of production-ready Grafana [dashboards]({{< relref "dashboards/_index.md" >}}) and alerts to monitor the state and health of a Fire cluster.

## Requirements

- Grafana Fire dashboards and alerts [require specific labels]({{< relref "requirements.md" >}}) to be set by Prometheus or Grafana Agent when scraping your Fire cluster profiles
- Some dashboards require recording rules that you should install in your Prometheus

## Install from package

Grafana Fire provides ready to use Grafana dashboards in the `.json` format and Prometheus alerts in the `.yaml` format, that you can directly import into your Grafana installation and Prometheus config.

The packaged dashboards and alerts have been compiled from the sources using a default configuration and don't allow you to customize the [required profiles label names]({{< relref "requirements.md" >}}).
If you need to customize the required profiles label names please choose one of the other installation options.

1. Download [dashboards](https://github.com/grafana/fire/tree/main/operations/fire-mixin-compiled/dashboards), [recording rules](https://github.com/grafana/fire/blob/main/operations/fire-mixin-compiled/rules.yaml) and [alerts](https://github.com/grafana/fire/blob/main/operations/fire-mixin-compiled/alerts.yaml) from Grafana Fire repository
2. [Import dashboards in Grafana](https://grafana.com/docs/grafana/latest/dashboards/export-import/#import-dashboard)
3. Install recording rules and alerts in your Prometheus

## Install from sources

Grafana Fire dashboards and alerts are built using [Jsonnet](https://jsonnet.org) language and you can compile them from sources.
If you choose this option, you can change the configuration to match your deployment, like customizing the [required label names]({{< relref "requirements.md" >}}).

1. Checkout Fire source code
   ```bash
   git clone https://github.com/grafana/fire.git
   ```
2. Review the mixin configuration at `operations/fire-mixin/config.libsonnet`, and apply your changes if necessary.
3. Compile the mixin
   ```bash
   make build-mixin
   ```
4. Import the dashboards saved at `operations/fire-mixin-compiled/dashboards/` in [Grafana](https://grafana.com/docs/grafana/latest/dashboards/export-import/#import-dashboard)
5. Install the recording rules saved at `operations/fire-mixin-compiled/rules.yaml` in your Prometheus
6. Install the alerts saved at `operations/fire-mixin-compiled/alerts.yaml` in your Prometheus

## Install dashboards from Jsonnet mixin

In case you're already using Jsonnet to define your infrastructure as a code, you can vendor the Grafana Fire mixin directly into your infrastructure repository and configure it overriding the `_config` fields.
Given the exact setup really depends on a case-by-case basis, the following instructions are not meant to be prescriptive but just show the main steps required to vendor the mixin.

1. Initialise Jsonnet
   ```bash
   jb init
   ```
2. Install Grafana Fire mixin
   ```bash
   jb install github.com/grafana/fire/operations/fire-mixin@main
   ```
3. Import and configure it
   ```jsonnet
   (import 'github.com/grafana/fire/operations/fire-mixin/mixin.libsonnet') + {
     _config+:: {
       // Override the Grafana Fire mixin config here.
     },
   }
   ```
