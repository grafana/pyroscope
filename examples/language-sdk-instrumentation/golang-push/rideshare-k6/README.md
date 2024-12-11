# Pyroscope k6 integration

This example demonstrates Pyroscope's k6 integration. This example requires k6
to be installed. To install k6, see: https://grafana.com/docs/k6/latest/set-up/install-k6/.

## Running the example

To run the example, use the following command to start all the necessary services.

```
docker compose up
```

This includes the following services:

- A Pyroscope server (http://localhost:4040)
- Grafana instance with Explore Profiles and a Pyroscope data source installed (http://localhost:3000)
- An nginx load balancer for the Rideshare services (http://localhost:5001)

> [!NOTE]
> We use nginx as a load balancer for the Rideshare services to simplify the
> load test script. Without nginx, the k6 load test would need to be aware of
> each Rideshare instance, resulting in tests with unnecessary boilerplate which
> would generally not be seen in the real world.

Once Docker Compose is running, use the following command to execute a k6 load
test against the Rideshare service.

```
k6 run load.js
```

Finally, navigate to http://localhost:3000 and open the Explore Profiles app. After a small delay, k6 test metadata should begin to appears as labels on the
"ride-sharing-app" application tile.
