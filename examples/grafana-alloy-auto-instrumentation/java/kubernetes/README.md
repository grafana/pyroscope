# Grafana Alloy Java profiling via auto-instrumentation in Kubernetes

This repository provides a practical demonstration of leveraging Grafana Alloy for continuous Java application profiling using Pyroscope in Kubernetes. 
It illustrates a seamless approach to profiling Java processes, aiding in performance optimization.

## Overview

Grafana Alloy automates Java process discovery for profiling, streamlining the setup for applications. It enables precise and targeted profiling configurations through the Grafana Alloy settings.

Java profiling via Grafana Alloy is based on a few components:
- `discovery.process` for process discovery
- `discovery.kubernetes` for adding Kubernetes labels (namespace, pod, and more)
- `discovery.relabel` for detecting java processes and setting up labels
- `pyroscope.java` for enabling profiling for specific applications
- `pyroscope.write` for writing the profiles data to a remote endpoint

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/java/) for an in-depth understanding and additional configuration options for Java profiling with Grafana Alloy.
Also, check the [Grafana Alloy Components reference](https://grafana.com/docs/alloy/latest/reference/components/) for more details on each used component.

### async-profiler

The `pyroscope.java` component internally uses the [async-profiler](https://github.com/async-profiler/async-profiler) library.
This approach offers a key advantage over other instrumentation mechanisms in that you can profile applications that are already running without interruptions (code changes, config changes or restarts).

Under the hood, this is achieved by attaching to the application at a process level and issuing commands to control profiling.

## Getting started

To use this example:

1. Set up a local kubernetes cluster using Kind or a similar tool.
2. Clone this repository and navigate to this example's directory.
3. Create a `pyroscope-java` namespace:
    ```shell
        kubectl create namespace pyroscope-java
    ```
4. Deploy the manifests:
    ```shell
       kubectl apply -n pyroscope-java -f .
    ```

After the deployment is operational, the Grafana Alloy will profile the Java application using the defined configuration.
The example will deploy a Grafana instance in the same cluster, available via the `grafana` service at port 3000.

### Observe profiling data

You can open grafana in your browser by port-forwarding traffic to the service:
```shell
kubectl port-forward -n pyroscope-java deployment/grafana 3000 3000
```

Now that everything is set up, you can browse profiling data through the [Explore profiles app](http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer).

![image](https://github.com/user-attachments/assets/16f5559a-0bbc-4cf3-9589-fa4374bbc7e8)
![image](https://github.com/user-attachments/assets/ca28d228-93c3-4e16-a63c-285005c7b203)


## Documentation

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/java/) for an in-depth understanding and additional configuration options for Java profiling with Grafana Alloy.
