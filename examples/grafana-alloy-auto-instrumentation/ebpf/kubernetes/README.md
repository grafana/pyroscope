# Grafana Alloy eBPF profiling via auto-instrumentation in Kubernetes

This repository provides a practical demonstration of leveraging Grafana Alloy for continuous application profiling 
using eBPF and Pyroscope in Kubernetes. It illustrates a seamless approach to profiling Golang and Python processes, 
aiding in performance optimization.

## Overview

eBPF profiling via Grafana Alloy is based on a few components:
- `discovery.kubernetes` for discovering Kubernetes pods
- `discovery.relabel` for detecting and filtering target processes and setting up labels
- `pyroscope.ebpf` for enabling eBPF profiling for specific applications
- `pyroscope.write` for writing the profiles data to a remote endpoint

Refer to the [official documentation](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/) for an in-depth understanding and additional configuration options for eBPF with Grafana Alloy.
Also, check the [Grafana Alloy Components reference](https://grafana.com/docs/alloy/latest/reference/components/) for more details on each used component.



## Getting started

To use this example:

1. Set up a local kubernetes cluster using Kind or a similar tool.
2. Clone this repository and navigate to this example's directory.
3. Deploy the manifests:
    ```shell
       kubectl apply -f alloy.yaml -f grafana.yaml -f pyroscope.yaml -f python-fast-slow.yaml
    ```
4. Port-forward the Grafana service to access the Explore Profiles app:
    ```shell
       kubectl port-forward -n pyroscope-ebpf service/grafana  3000:3000
    ```
5. Explore profiles http://localhost:3000/a/grafana-pyroscope-app/profiles-explore

After the deployment is operational, the Grafana Alloy will profile the Go and Python applications using `pyroscope.ebpf` component.

## Documentation

Refer to the [official documentation](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/) for an in-depth understanding and additional configuration options for eBPF profiling with Grafana Alloy.
