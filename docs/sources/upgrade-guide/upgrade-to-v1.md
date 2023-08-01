---
title: "Upgrade to Grafana Pyroscope v1.0"
menuTitle: "Upgrade to v1.0"
description: "Upgrading to Pyroscope v1.0"
weight: 90
keywords:
  - pyroscope
  - phlare
  - upgrade
  - upgrading
---

# Upgrade to v1.0

Version 1.0 of Pyroscope is a major release that includes breaking changes. This guide explains how to upgrade to v1.0 from previous versions.

### New Architecture

We're excited to announce the main change to Pyroscope since its acquisition by Grafana Labs: a new horizontally scalable architecture. Our team took unique learnings that we have gained over the years about profiling data and combined it with a battle-tested Cortex architecture that powers other Grafana Labs databases such as Loki, Mimir or Tempo.

This means that Pyroscope is now equipped to support large deployments with many high-cardinality labels and unlock more features and use cases.

## Breaking Changes

Making big leaps means that we have to break some things. We've tried to minimize the impact of these changes as much as possible, but some of them are unavoidable. We apologize for any inconvenience this may cause. We encourage you to contribute to the community by creating new issues or upvoting existing ones with an `og-feature` label in the [Pyroscope GitHub repository](https://github.com/grafana/pyroscope/labels/og-feature).

### Changes in the configuration file

TODO: list common things people change in the config file, provide examples of how to change them in the new config file

### Dropping support for certain subcommands

We stripped the pyroscope CLI of all subcommands that were related to the client side of profiling and only kept the ones that are related to the server side. This means that the following subcommands are no longer supported:
* `pyroscope exec`
* `pyroscope adhoc`
* `pyroscope connect`
* `pyroscope agent`

This strategic shift has been in the works for some time as we transition away from CLI-based profiling towards embracing native integrations. Moving forward, we encourage users to take advantage of native integrations tailored to specific programming languages, such as pip packages for Python, .NET packages for .NET applications, ruby gems for Ruby applications, and other appropriate tools.

By adopting native integrations, we aim to provide users with a more streamlined and efficient profiling experience, leveraging language-specific tools and libraries to deliver better performance, ease of use, and seamless integration with their respective applications.

### Dropping OAuth support

OAuth was implemented with SQLite, which made sense when Pyroscope was a single binary, but no longer makes sense with the new distributed architecture. Transitioning to a new distributed architecture means that we had to drop support for OAuth.

Going forwar, our recommendation is to use Grafana for visualizing your profiling data. Grafana 10 comes with a native support for Pyroscope and it supports OAuth and many other authentication methods.

## Community Call To Action

Pyroscope v1.0 comes with a huge architectural change and we've tried to minimize the impact of these changes as much as possible, but some of them are unavoidable. Our goal is for our users to have as smooth of a transition as possible. Therefore, we encourage you to contribute to the community by creating new issues or upvoting existing ones with an `og-feature` label in the [Pyroscope GitHub repository](https://github.com/grafana/pyroscope/labels/og-feature). Thank you for your feedback and engagement, which play a crucial role in shaping the future of Pyroscope.

