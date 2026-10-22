# .NET Example with Alloy and an HTTP Proxy

This example shows how to send profiles from a .NET application to Pyroscope through a forward HTTP proxy.

The Pyroscope .NET SDK does not support HTTP proxies. It ignores `HTTP_PROXY` and `HTTPS_PROXY`. To send profiles through a proxy, the application pushes profiles to a local Grafana Alloy instance, and Alloy forwards them to Pyroscope through the proxy.

## Architecture

- The `app` service is the [fast-slow](../fast-slow) application. It pushes profiles to Alloy at `http://alloy:9999`.
- Alloy receives profiles with `pyroscope.receive_http` and forwards them with `pyroscope.write` through Squid.
- Squid is the forward proxy. It is the only service connected to both networks.
- Alloy and the app are on the `edge` network only, so Alloy cannot connect to Pyroscope directly.
- Grafana visualizes the profiling data.

## Configuration

```alloy
pyroscope.receive_http "default" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9999
    }
    forward_to = [pyroscope.write.backend.receiver]
}

pyroscope.write "backend" {
    endpoint {
        url = "http://pyroscope:4040"
        proxy_url = "http://squid:3128"
    }
}
```

`proxy_from_environment` is `false` by default, so Alloy does not use `HTTP_PROXY` or `HTTPS_PROXY` unless you set it. Set `proxy_url`, or set `proxy_from_environment = true` and the proxy environment variables on the Alloy container.

To send profiles to Grafana Cloud, change `url` to your Grafana Cloud Profiles URL and add a `basic_auth` block.

## Running the example

```bash
docker compose up --build
```

Make sure that Squid forwards the requests:

```bash
docker compose logs squid | grep PusherService
```

Navigate to [Grafana](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=fast-slow-alloy-proxy.dotnet.app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds) to explore profiles.

```bash
docker compose down
```
