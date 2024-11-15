# Rideshare Example with Alloy Profiling

This example demonstrates how to use Alloy to receive and forward profiles from the rideshare example application.

## Architecture

- Regional services (us-east, eu-north, ap-south) send profiles to Alloy
- Alloy receives profiles on port 9999 and forwards them to Pyroscope
- Pyroscope stores and processes profiles
- Grafana visualizes the profiling data

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
    }
    external_labels = {
        "env" = "home-lab",
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
