---
title: "Grafana Fire documentation"
menuTitle: "Grafana Fire"
weight: 1
keywords:
  - Grafana Fire
  - Grafana profiles
  - time series database
  - TSDB
  - Prometheus storage
  - Prometheus remote write
  - profiles storage
  - profiless datastore
  - observability
---

# Grafana Fire documentation

![Grafana Fire](fire-logo.png)


# Operator and user guide

> Get started + video = Cyril
> Deploy on Kubernetes = Christian
> Play in Grafana
> Architecture = Christian
  > About the architecture
  > Deployment mode
  > Components
  > Block Format
  > Hash rings
  > Memberlist and gossip protocol
> Configuration = Cyril
  > About configuration (File vs Args / config map rolling out...)
  > Configure agent
    > About the agent (pull mode and only pprof)
    > Language support (build demo)
      > Go
      > Rust = Christian
      > Python = Christian
      > JVM = Cyril
      > NodeJS = Cyril
    > Comming soon Grafana Agent support
    > System wide and ebpf (comming soon)
  > Configure object storage
  > Configure on disk storage
  > About tenant id
  > Configuring memberlist (+DNS service discovery)
  > About Grafana Mimir IP address logging of a reverse proxy
  > Configuration parameters (all of it)
  > Configure Tracing
> Monitor Fire (coming soon)
> Run in production (coming soon)
> Firecli (create an issue for unified cli)
> Reference HTTP API (comming soon unstable talk about connect)
> Reference Glossary
