# Pyroscope k6 integration

This example demonstrates Pyroscope's k6 integration. This example requires k6
to be installed. To install k6, see: https://grafana.com/docs/k6/latest/set-up/install-k6/.

## Running the example

To run the example, use

```
docker compose up
```

This will start all the necessary services, including:

- A Pyroscope server (http://localhost:4040)
- Grafana instance with Explore Profiles and a Pyroscope data source installed (http://localhost:3000)
- An nginx load balancer for the Rideshare services (http://localhost:5001)

Once Docker Compose is running, use

```
k6 run load.js
```

to execute a k6 load test against the Rideshare service.
