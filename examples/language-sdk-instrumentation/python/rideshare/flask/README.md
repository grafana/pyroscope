## Flask Example

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

Navigate to Grafana to explore the generated profiles:

- [CPU](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds)
- [Allocated space](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=memory:alloc_space:bytes:space:bytes)
- [Space in use](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=memory:inuse_space:bytes:space:bytes)

The rideshare workload retains a bounded window of allocations. Filter by the
`vehicle` label to compare memory behavior across the bike, scooter, and car
endpoints.
