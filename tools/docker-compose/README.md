# docker-compose

This directory contains a `docker-compose.yml` file that can be used to start a local Grafana Pyroscope instance
with a Grafana instance. The `docker-compose.yml` file is configured to use the latest version of Grafana Pyroscope and Grafana.

To start Grafana Pyroscope and Grafana, run the following command:

```bash
docker-compose up -d
```

Grafana should be available at http://localhost:3000 and Grafana Pyroscope should be available at http://localhost:4040.

By default the Grafana Pyroscope datasource is already configured in Grafana.

The data will be persisted in the `data` directory.

To bring down the whole deployment, run the following command:

```bash
docker-compose down
```
