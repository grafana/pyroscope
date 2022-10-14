---
aliases:
  - /docs/phlare/latest/operators-guide/configuring/about-configurations/
description: Learn about Grafana Phlare configuration and key guidelines to consider.
menuTitle: About configurations
title: About Grafana Phlare configurations
weight: 10
---

# About Grafana Phlare configurations

You can configure Grafana Phlare via a ([YAML](https://en.wikipedia.org/wiki/YAML)-based) configuration file or CLI (command-line-interface) flags. It is best to specify your configuration via the configuration file rather than CLI flags. Every parameter that is set in the configuration file can also be set via a corresponding CLI flag. If you specify both CLI flags and configuration parameters, CLI flags take precedence over corresponding values in a configuration file. You can specify the configuration file by using the `-config.file` CLI flag.

To see the CLI flags that you need to get started with Grafana Phlare, run the `phlare -help` command.

To see the current configuration state of any component, use the `/api/v1/status/config` HTTP API endpoint.


## Operational considerations

Use a single configuration file, and either pass it to all replicas of Grafana Phlare (if you are running multiple single-process Phlare replicas) or to all components of Grafana Phlare (if you are running Grafana Phlare as microservices). If you are running Grafana Phlare on Kubernetes, you can achieve this by storing the configuration file in a [ConfigMap](https://kubernetes.io/docs/concepts/configuration/configmap/) and mounting it in each Grafana Phlare container.

This recommendation helps to avoid a common misconfiguration pitfall: while certain configuration parameters might look like they’re only needed by one type of component, they might in fact be used by multiple components. For example, the `-ingester.ring.replication-factor` CLI flag is not only required by ingesters, but also by distributors, queriers.

By using a single configuration file, you ensure that each component gets all of the configuration that it needs without needing to track which parameter belongs to which component.
There is no harm in passing a configuration that is specific to one component (such as an ingester) to another component (such as a querier). In such case, the configuration is simply ignored.

If you need to, you can use advanced CLI flags to override specific values on a particular Grafana Phlare component or replica. This can be helpful if you want to change a parameter that is specific to a certain component, without having to do a full restart of all other components.

The most common use case for CLI flags is to use the `-target` flag to run Grafana Phlare as microservices. By setting the `-target` CLI flag, all Grafana Phlare components share the same configuration file, but you can make them behave as a given component by specifying a `-target` command-line value, such as `-target=ingester` or `-target=querier`.
