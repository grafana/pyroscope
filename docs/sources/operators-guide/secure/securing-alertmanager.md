---
aliases:
  - /docs/mimir/latest/operators-guide/securing/securing-alertmanager/
description: Learn how to secure the Alertmanager.
menuTitle: Securing Alertmanager
title: Securing Grafana Mimir Alertmanager
weight: 40
---

# Securing Grafana Mimir Alertmanager

By default, the Alertmanager exposes API endpoints that enable a user to configure the Alertmanager.
The Alertmanager configuration includes receivers that create network connections to send the alerting notifications.
For example, the `webhook` receiver enables a user to configure an arbitrary URL to which the Alertmanager sends a customizable request for every alerting notification.
If the Alertmanager network security is not hardened, Grafana Mimir users might configure the Alertmanager to issue requests to any network address both in the local network and the Internet.

We recommend hardening the network on which the Alertmanager runs.
Although hardening the network is out of the scope for Grafana Mimir, Grafana Mimir provides a basic built-in firewall that blocks connections created by Alertmanager receivers:

- To block specific network addresses in Alertmanager receivers, set `-alertmanager.receivers-firewall-block-cidr-networks` to a comma-separated list of network CIDRs to block.
- To block private and local addresses in Alertmanager receivers, set `-alertmanager.receivers-firewall-block-private-addresses=true`.

You can override the Alertmanager built-in firewall settings on a per-tenant basis in the overrides section of the [runtime configuration]({{< relref "../configure/about-runtime-configuration.md" >}}).

> **Note:** You can disable the Alertmanager configuration API by setting `-alertmanager.enable-api=false`.
