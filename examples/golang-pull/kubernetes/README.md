# Pyroscope pull mode and Kubernetes Discovery

This example demonstrates how Pyroscope can be used to scrape pprof profiles from Kubernetes pods.

### 1. Run minikube kubernetes cluster locally (optional)

In this example we use minikube for sake of simplicity, visit [minikube documentation](https://minikube.sigs.k8s.io/docs/start/)
to learn more.

```shell
minikube start
```

### 2. Install Pyroscope with Helm chart

The official [Pyroscope Helm chart](https://github.com/pyroscope-io/helm-chart) deploys Pyroscope server and creates proper RBAC roles:

```shell
helm repo add grafana https://grafana.github.io/helm-chart
helm install pyroscope grafana/pyroscope --version v1.0.0-rc.0
```

### 3. Install Grafana Agent with Helm chart

The official [Pyroscope Helm chart](https://github.com/pyroscope-io/helm-chart) deploys Pyroscope server and creates proper RBAC roles:

```shell
helm install agent grafana/grafana-agent -f values.yaml
```

Note that we apply configuration defined in `values.yaml`: Grafana Agent supports many different ways of discovering targets, in this example we use Kubernetes Service Discovery:

```yaml
agent:
  # -- Mode to run Grafana Agent in. Can be "flow" or "static".
  mode: 'flow'
  configMap:
    # -- Create a new ConfigMap for the config file.
    create: true
    # -- Content to assign to the new ConfigMap.  This is passed into `tpl` allowing for templating from values.
    content: |
      logging {
        level = "debug"
        format = "logfmt"
      }

      discovery.kubernetes "pyroscope_kubernetes" {
        role = "pod"
      }

      pyroscope.write "example" {
        // Send metrics to a locally running Pyroscope instance.
        endpoint {
          url = "http://pyroscope:4040"

          // To send data to Grafana Cloud you'll need to provide username and password.
          // basic_auth {
          //   username = "myuser"
          //   password = "mypassword"
          // }
        }
        external_labels = {
          "env" = "example",
        }
      }

      pyroscope.scrape "default" {
        targets = discovery.kubernetes.pyroscope_kubernetes.targets
        forward_to = [pyroscope.write.example.receiver]
      }
```

### 4. Deploy Hot R.O.D. application

As a sample application we use slightly modified Jaeger [Hot R.O.D.](https://github.com/jaegertracing/jaeger/tree/master/examples/hotrod) demo –
the only difference is that we enabled built-in Go `pprof` HTTP endpoints. You can find the modified code in the [hotrod-goland](https://github.com/pyroscope-io/hotrod-golang) repository.

```shell
kubectl apply -f manifests.yaml
```

### 5. Observe profiling data

Profiling is more fun when the application does some work. Let's order some rides in our Hot R.O.D. app:
```shell
minikube service hotrod-golang
```

Now that everything is set up, you can browse profiling data via Pyroscope UI:
```shell
minikube service pyroscope-pyroscope
```
