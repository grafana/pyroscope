# Django Example

This example runs Django with Gunicorn and preloads the application before Gunicorn forks its workers.
The Pyroscope Python client starts background threads and must not be initialized in the Gunicorn master process before those forks.

The [`post_fork` hook](app/gunicorn.conf.py) calls `pyroscope.configure()` separately in each worker.
Don't move that call into Django settings or another application module: Gunicorn imports those modules in the master process when `preload_app` is enabled.
For more information, refer to the [Python client documentation](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/#use-the-python-client-with-forked-processes).

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

Navigate to [Grafana](http://localhost:3000/a/grafana-pyroscope-app/explore?explorationType=flame-graph&var-serviceName=ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds) to explore profiles.
