# OpenTelemetry eBPF profiler examples

**⚠️ Important: Linux-only Support**
This example can only be run on Linux systems (amd64/arm64) as it relies on eBPF technology which is specific to the Linux kernel. The profiler requires privileged access to system resources.
For more details refer to the OpenTelemetry ebpf profiler [docs](https://github.com/open-telemetry/opentelemetry-ebpf-profiler).

These examples demonstrate:
1. OpenTelemetry eBPF profiler collecting system-wide profiles
2. OpenTelemetry Collector receiving and processing the data from the profiler
3. Pyroscope receiving and visualizing the profiles via Grafana

## Docker example
1. Start the environment:

```bash
# Start all services
docker compose up --build

# To clean up
docker compose down
```
2. Access the UI:
```bash
# Access Grafana
http://localhost:3000
```

## Kubernetes example

1. Build and prepare the profiler image:

```bash
# Build the image with the binary
docker build -t test-ebpf-profiler:latest .

# Make the image available if necessary. e.g in Minikube
minikube image load test-ebpf-profiler:latest
```
2. Deploy to Kubernetes:
```bash
# Apply the manifests
kubectl apply -f .

# Clean up
kubectl delete -f .
```
3. Access the UI:
```bash
# Port forward Grafana
kubectl port-forward svc/grafana-service 3000:3000

# Access Grafana
http://localhost:3000
```

## Example output:
![Image](https://github.com/user-attachments/assets/15ff58d4-218a-43dd-9835-df12e13ced3f)
