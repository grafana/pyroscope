---
title: "Grafana Mimir query-tee"
menuTitle: "Query-tee"
description: "Use query-tee to compare query results and performance between two Grafana Mimir clusters."
weight: 30
---

# Grafana Mimir query-tee

The query-tee is a standalone tool that you can use for testing purposes when comparing the query results and performances of two Grafana Mimir clusters.
The two Mimir clusters compared by the query-tee must ingest the same series and samples.

The query-tee exposes Prometheus-compatible read API endpoints and acts as a proxy.
When the query-tee receives a request, it performs the same request against the two backend Grafana Mimir clusters and tracks the response time of each backend, and compares the query results.

## Download the query-tee

- Using Docker:

```bash
docker pull "grafana/query-tee:latest"
```

- Using a local binary:

Download the appropriate [release asset](https://github.com/grafana/mimir/releases/latest) for your operating system and architecture and make it executable.

For Linux with the AMD64 architecture, execute the following command:

```bash
curl -Lo query-tee https://github.com/grafana/mimir/releases/latest/download/query-tee-linux-amd64
chmod +x query-tee
```

## Configure the query-tee

The query-tee requires the endpoints of the backend Grafana Mimir clusters.
You can configure the backend endpoints by setting the `-backend.endpoints` flag to a comma-separated list of HTTP or HTTPS URLs.

For each incoming request, the query-tee clones the request and sends it to each configured backend.

> **Note:** You can configure the query-tee proxy listening ports via the `-server.http-service-port` flag for the HTTP port and `server.grpc-service-port` flag for the gRPC port.

## How the query-tee works

This section describes how the query-tee tool works.

### API endpoints

Query-tee accepts two types of requests:

1. HTTP requests on the configured `-server.http-service-port` flag (default port 80)
1. [HTTP over gRPC](https://github.com/weaveworks/common/tree/master/httpgrpc) requests on the configured `-server.grpc-service-port` flag (default port: 9095)

The following Prometheus API endpoints are supported by `query-tee`:

- `GET <prefix>/api/v1/query`
- `GET <prefix>/api/v1/query_range`
- `GET <prefix>/api/v1/query_exemplars`
- `GET <prefix>/api/v1/labels`
- `GET <prefix>/api/v1/label/{name}/values`
- `GET <prefix>/api/v1/series`
- `GET <prefix>/api/v1/metadata`
- `GET <prefix>/api/v1/alerts`
- `GET <prefix>/prometheus/config/v1/rules`

You can configure the `<prefix>` by setting the `-server.path-prefix` flag, which defaults to an empty string.

### Pass-through requests

The query-tee can optionally act as a transparent proxy for requests to routes not matching any of the supported API endpoints.
You can enable the pass-through support setting `-proxy.passthrough-non-registered-routes=true` and configuring a preferred backend using the `-backend.preferred` flag.
When pass-through is enabled, a request for an unsupported API endpoint is transparently proxied to the configured preferred backend.

### Authentication

The query-tee supports [HTTP basic authentication](https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication).
The query-tee can merge the HTTP basic authentication in the received request with the username and configured in a backend URL.

A request sent from the query-tee to a backend includes HTTP basic authentication when one of the following conditions is met:

- If the backend endpoint URL is configured with both a username and password, then query-tee uses it.
- If the backend endpoint URL is configured only with a username, then query-tee keeps the configured username and injects the password received in the incoming request.
- If the backend endpoint URL is configured without a username and password, then query-tee forwards the authentication credentials found in the incoming request.

### Backend response selection

The query-tee enables you to configure a preferred backend that selects the response to send back to the client.
The preferred backend can be configured via `-backend.preferred=<hostname>`.
The value of the preferred backend configuration option must be the hostname of one of the configured backends.

When a preferred backend is configured, the query-tee uses the following algorithm to select the backend response to send back to the client:

1. If the preferred backend response status code is 2xx or 4xx, the query-tee selects the response from the preferred backend.
1. If at least one backend response status code is 2xx or 4xx, the query-tee selects the first received response whose status code is 2xx or 4xx.
1. If no backend response status code is 2xx or 4xx, the query-tee selects the first received response regardless of the status code.

When a preferred backend is not configured, the query-tee uses the following algorithm to select the backend response to send back to the client:

1. If at least one backend response status code is 2xx or 4xx, the query-tee selects the first received response whose status code is 2xx or 4xx.
1. If no backend response status code is 2xx or 4xx, the query-tee selects the first received response regardless of the status code.

> **Note:** The query-tee considers a 4xx response as a valid response to select because a 4xx status code generally means the error is caused by an invalid request and not due to a server side issue.

### Backend results comparison

The query-tee can optionally compare the query results received by two backends.
The query results comparison can be enabled setting the flag `-proxy.compare-responses=true` and requires that:

1. Two backends have been configured setting `-backend.endpoints`.
1. A preferred backend is configured setting `-backend.preferred`.

When the query results comparison is enabled, the query-tee compares the response received from the two configured backends and logs a message for each query whose results don't match. Query-tee keeps track of the number of successful and failed comparison through the metric `cortex_querytee_responses_compared_total`.

> **Note**: Floating point sample values are compared with a tolerance that can be configured via `-proxy.value-comparison-tolerance`. The configured tolerance prevents false positives due to differences in floating point values rounding introduced by the non-deterministic series ordering within the Prometheus PromQL engine.

### Exported metrics

The query-tee exposes the following Prometheus metrics at the `/metrics` endpoint listening on the port configured via the flag `-server.metrics-port`:

```bash
# HELP cortex_querytee_backend_request_duration_seconds Time (in seconds) spent serving requests.
# TYPE cortex_querytee_backend_request_duration_seconds histogram
cortex_querytee_backend_request_duration_seconds_bucket{backend="<hostname>",method="<method>",route="<route>",status_code="<status>",le="<bucket>"}
cortex_querytee_backend_request_duration_seconds_sum{backend="<hostname>",method="<method>",route="<route>",status_code="<status>"}
cortex_querytee_backend_request_duration_seconds_count{backend="<hostname>",method="<method>",route="<route>",status_code="<status>"}

# HELP cortex_querytee_responses_total Total number of responses sent back to the client by the selected backend.
# TYPE cortex_querytee_responses_total counter
cortex_querytee_responses_total{backend="<hostname>",method="<method>",route="<route>"}

# HELP cortex_querytee_responses_compared_total Total number of responses compared per route name by result.
# TYPE cortex_querytee_responses_compared_total counter
cortex_querytee_responses_compared_total{route="<route>",result="<success|fail>"}
```

### Ruler remote operational mode test

When the ruler is configured with the [remote evaluation mode]({{< relref "../architecture/components/ruler/index.md" >}}) you can use the query-tee to compare rule evaluations too.
To test ruler evaluations with query-tee, set the `-ruler.query-frontend.address` CLI flag or its respective YAML configuration parameter for the ruler with query-tee's gRPC address:

```
ruler:
  query_frontend:
    address: "dns://query-tee:9095"
```

When the ruler evaluates a rule, the test flow is the following:

1. ruler sends gRPC request to query-tee
1. query-tee forwards the request to the query-frontend backends configured setting the `-backend.endpoints` CLI flag
1. query-tee receives the response from the query-frontend and forwards the result (based on the preferred backend) to the ruler
