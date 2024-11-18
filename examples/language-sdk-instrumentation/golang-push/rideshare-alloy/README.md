# Rideshare Example with Alloy Profiling

This example demonstrates how to use Alloy to receive and forward profiles from the rideshare example application.

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
docker pull grafana/alloy:latest

# Run the example
docker-compose up --build

# Reset if needed
docker-compose down
```
Access services:

- Grafana: http://localhost:3000
- Pyroscope: http://localhost:4040
