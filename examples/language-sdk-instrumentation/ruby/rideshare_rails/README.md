# Rails Rideshare Example

To run the example run the following commands:
```
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

# Run the example project:
docker compose up --build

# Reset the database (if needed):
docker compose down
```

Navigate to [Grafana](http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds) to Explore Profiles.
