# Rideshare Example with Alloy Profiling

This example demonstrates how to use Alloy to receive and forward profiles from the rideshare example application.

To learn more about the `pyroscope.receive_http` component in Alloy, refer to the [`receive_profiles`](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/receive_profiles/) documentation.

## Architecture

- Regional services (us-east, eu-north, ap-south) push profiles to Alloy
- Alloy receives profiles on port 9999 and forwards them to Pyroscope
- Pyroscope stores and processes profiles
- Grafana visualizes the profiling data

![Pyroscope agent server diagram](https://grafana.com/media/docs/pyroscope/pyroscope_client_server_diagram_11_18_2024.png)

## Configuration

The example uses this Alloy configuration:
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
        // url = "<Grafana Cloud URL>"
        // basic_auth {
        //     username = "<Grafana Cloud User>"
        //     password = "<Grafana Cloud Password>"
        // }
    }
    external_labels = {
        "env" = "production",
    }
}
```

## Running the example
```bash
# Pull latest images
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest
docker pull grafana/grafana/alloy-dev:latest

# Run the example
docker-compose up --build

# Reset if needed
docker-compose down
```

Navigate to [Grafana](http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds) to Explore Profiles.
