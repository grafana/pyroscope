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

### Querying the new API manually

[Demo](https://github.com/grafana/pyroscope/assets/12090599/46b5560b-003b-4009-8767-0ee53833d06b)

1. Navigate to the Explore view and open a sample trace. If traces are not available, make sure all the containers are running.
2. In the trace view, find `ride-sharing-app` service spans.
3. Specify span identifiers (one or more) in the `spanSelector` field of the request.
4. Specify proper `start` and `end` timestamps (milliseconds).

Please note that the pyroscope SDK is configured to only record root spans, therefore stack trace samples
of child spans are included into the root span profile. These spans are marked with `pyroscope.profiling.enabled`
attribute set to `true`.

#### cURL

```shel
curl \
  -X POST http://localhost:4040/querier.v1.QuerierService/SelectMergeSpanProfile \
  -H 'Content-type: application/json; charset=UTF-8' \
  --data-binary @- << EOF
{
    "start": 1696245000000,
    "end": 1696247750000,
    "profileTypeID": "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
    "labelSelector": "{service_name=\"ride-sharing-app\"}",
    "spanSelector": ["5bde01274734d594"]
}
EOF
```

#### JS (_mind CORS_)

```js
fetch('http://localhost:4040/querier.v1.QuerierService/SelectMergeSpanProfile', {
  method: 'POST',
  body: JSON.stringify({
    profileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
    start: 1696245000000,
    end: 1696247750000,
    labelSelector: "{service_name=\"ride-sharing-app\"}",
    spanSelector: ["5bde01274734d594"], 
  }),
  headers: {
    'Content-type': 'application/json; charset=UTF-8'
  }
})
.then(res => res.json())
.then(console.log) 
```
