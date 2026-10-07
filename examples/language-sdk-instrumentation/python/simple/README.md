## Basic Example

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

- [CPU](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=simple.python.app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds)
- [Allocated space](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=simple.python.app&var-profileMetricId=memory:alloc_space:bytes:space:bytes)
- [Space in use](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=simple.python.app&var-profileMetricId=memory:inuse_space:bytes:space:bytes)

The application continuously allocates 64 KiB chunks and retains a bounded
rotating window. This produces useful allocation and in-use profiles without
allowing the example's memory use to grow indefinitely.
