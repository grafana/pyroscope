---
title: Upgrade your Grafana Pyroscope installation
menuTitle: Upgrade
description: Upgrade your Pyroscope installation to the latest version.
weight: 350
keywords:
  - pyroscope
  - phlare
  - upgrade
  - upgrading
---

# Upgrade your Grafana Pyroscope installation

You can upgrade an existing Pyroscope installation to the next version.
However, any new release has the potential to have breaking changes that should be tested in a non-production environment prior to rolling these changes to production.

The upgrade process changes for each version, depending upon the changes made for the subsequent release.

This upgrade guide applies to on-premise installations and not for Grafana Cloud.

## Upgrade to Pyroscope 1.0

Version 1.0 of Pyroscope is a major release that includes breaking changes.
This guide explains how to upgrade to v1.0 from previous versions.

This document describes in detail the changes that we've made to Pyroscope and how they affect you. For convenience, at the end of this guide we provide short checklists for you to follow.

### New architecture

We're excited to announce the main change to Pyroscope since its acquisition by Grafana Labs: a new horizontally scalable architecture.
Our team took unique learnings that we have gained over the years about profiling data and combined it with a battle-tested Cortex architecture that powers other Grafana Labs databases such as Loki, Mimir, and Tempo.
This means you can now provision Pyroscope as a highly available service backed by cheap object storage, with the ability to scale up and down as needed.

### License change

Pyroscope server is now licensed under the [AGPLv3](https://opensource.org/license/agpl-v3/). All of our client integrations are still licensed under the [Apache 2.0 license](https://opensource.org/license/apache-2-0/).

Pyroscope was founded in 2020 to build a sustainable business around the open source Pyroscope project, so that revenue from our commercial offerings could be re-invested in the technology and the community.

We believe that the AGPLv3 license is the best way forward for Pyroscope. It allows us to continue to build a sustainable business around Pyroscope, while also ensuring that the project remains open source and that the community can continue to use and contribute to it.

### New Docker repository

The new Pyroscope Docker repository is located at [grafana/pyroscope](https://hub.docker.com/r/grafana/pyroscope). The old repository at [pyroscope/pyroscope](https://hub.docker.com/r/pyroscope/pyroscope) will no longer be updated.

### Breaking changes

Making big leaps means that we have to break some things. We've tried to minimize the impact of these changes as much as possible, but some of them are unavoidable. We apologize for any inconvenience this may cause. We encourage you to contribute to the community by creating new issues or upvoting existing ones with an `og-feature` label in the [Pyroscope GitHub repository](https://github.com/grafana/pyroscope/labels/og-feature).

#### Storage format changes

The new local storage format is entirely new, optimized for object storage. We do not support migrating from the old storage format to the new one. This means that you will lose data when upgrading to v1.0.

#### Configuration file changes

The configuration file parameters as well as the default location for the configuration file have changed. The old config file is usually located at `/etc/pyroscope/server.yml` and the new config file is at `/etc/pyroscope/config.yaml`. You can find detailed descriptions of all configuration parameters [here]({{< relref "../configure-server/reference-configuration-parameters" >}}).

#### Dropping support for certain subcommands

We stripped the pyroscope CLI of all subcommands that were related to the client side of profiling and only kept the ones that are related to the server side. This means that the following subcommands are no longer supported:
* `pyroscope exec`
* `pyroscope adhoc`
* `pyroscope connect`
* `pyroscope agent`

This strategic shift has been in the works for some time as we transition away from CLI-based profiling towards embracing native integrations.
Moving forward, we encourage users to take advantage of native integrations tailored to specific programming languages, such as pip packages for Python, .NET packages for .NET applications, Ruby gems for Ruby applications, and so on.
The [eBPF integration](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/ebpf/) is also a good way to get profiling data for your whole cluster.

By adopting native integrations, we aim to provide users with a more streamlined and efficient profiling experience, leveraging language-specific tools and libraries to deliver better performance, ease of use, and seamless integration with their respective applications.

#### Dropping OAuth support

OAuth was implemented with SQLite, which made sense when Pyroscope was a single binary, but is no longer feasible with the new distributed architecture. Transitioning to a new distributed architecture means that we had to drop support for OAuth.

Our recommendation is to use Grafana for visualizing your profiling data. [Grafana 10](/docs/grafana/latest/whatsnew/whats-new-in-v10-0/) comes with native support for Pyroscope and very mature support for OAuth as well as many other authentication methods.

#### API stability

Even though this is a major release, the HTTP API is not yet stable and is subject to changes. Going forward we will do our best to keep the APIs backwards compatible and minimize the impact of these changes and provide a migration path.

### Community call to action

Pyroscope v1.0 comes with a huge architectural change and we've tried to minimize the impact of these changes as much as possible, but some of them are unavoidable. Our goal is for our users to have as smooth of a transition as possible. Therefore, we encourage you to contribute to the community by creating new issues or upvoting existing ones with an `og-feature` label in the [Pyroscope GitHub repository](https://github.com/grafana/pyroscope/labels/og-feature). Thank you for your feedback and engagement, which play a crucial role in shaping the future of Pyroscope.


### Upgrade Checklists for v1.0

We provide the following checklists to help you upgrade to v1.0.

#### Upgrade Checklist for Docker deployments

When upgrading to v1.0, we suggest that you follow this checklist:
* Migrate your configuration from the old format to the new format (old config is usually located at `/etc/pyroscope/server.yml` and the new config is at `/etc/pyroscope/config.yaml`). There's a detailed description of all configuration parameters [here]({{< relref "../configure-server/reference-configuration-parameters" >}}).
* Upgrade docker image from `pyroscope/pyroscope` to `grafana/pyroscope`. Link to the new docker image is [here](https://hub.docker.com/r/grafana/pyroscope).
* Delete old data (typically found at `/var/lib/pyroscope`).

#### Upgrade Checklist for Helm deployments

When upgrading to v1.0, we suggest that you follow this checklist:

* Migrate your configuration from the old format to the new format (old config is usually located at `/etc/pyroscope/server.yml` and the new config is at `/etc/pyroscope/config.yaml`). There's a detailed description of all configuration parameters [here]({{< relref "../configure-server/reference-configuration-parameters" >}}).
* Delete the old Helm chart:
  ```bash
  helm delete pyroscope # replace pyroscope with the name you used when installing the chart
  ```
* Install the new Helm chart:
  ```bash
  kubectl create namespace pyroscope
  helm repo add grafana https://grafana.github.io/helm-charts
  helm repo update
  helm -n pyroscope install pyroscope grafana/pyroscope
  ```
  For more information on how to install the Helm chart, see our Helm documentation [here]({{< relref "../deploy-kubernetes" >}}).
