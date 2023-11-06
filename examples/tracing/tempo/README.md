### This is a pre-release demo project for internal use only

The docker compose consists of:
 - `rideshare` demo application instrumented with OpenTelemetry and Pyroscope SDK
 - Tempo
 - Pyroscope
 - Grafana

`rideshare` applications generate traces and profiling data that should be available in Grafana.
Pyroscope and Tempo datasources are provisioned automatically.

### Build and run

The project can be run locally with the following commands:

```shell
GOOS=linux GOARCH=amd64 make build -C ../../..
docker-compose up --build
```

Pyroscope and the demo application will be built from the current branch.  After the release, this will be changed so that the latest Pyroscope docker image is pulled from the Grafana repo.
