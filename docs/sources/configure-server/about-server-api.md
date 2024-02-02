---
description: Learn about the Pyrocope server API
menuTitle: Server HTTP API 
title: Pyroscope server HTTP API
weight: 20
---

# Pyroscope server HTTP API

Pyroscope server exposes an HTTP API for querying profiling data and ingesting profiling data from other sources.

## Authentication

Grafana Pyroscope doesn't include an authentication layer. Operators should use an authenticating reverse proxy for security.

In multi-tenant mode, Pyroscope requires the X-Scope-OrgID HTTP header set to a string identifying the tenant. This responsibility should be handled by the authenticating reverse proxy. For more information, refer to the [multi-tenancy documentation]({{< relref "./about-tenant-ids" >}}).

## Ingestion

There is one primary endpoint: POST /ingest. It accepts profile data in the request body and metadata as query parameters.

The following query parameters are accepted:

| Name               | Description                             | Notes                          |
|:-------------------|:----------------------------------------|:-------------------------------|
| `name`             | application name                        | required                       |
| `from`             | UNIX time of when the profiling started | required                       |
| `until`            | UNIX time of when the profiling stopped | required                       |
| `format`           | format of the profiling data            | optional (default is `folded`) |
| `sampleRate`       | sample rate used in Hz                  | optional (default is `100` Hz) |
| `spyName`          | name of the spy used                    | optional                       |
| `units`            | name of the profiling data unit         | optional (default is `samples` |
| `aggregrationType` | type of aggregration to merge profiles  | optional (default is `sum`)    |


`name` specifies application name. For example:
```
my.awesome.app.cpu{env=staging,region=us-west-1}
```

The request body contains profiling data, and the Content-Type header may be used alongside format to determine the data format.

Some of the query parameters depend on the format of profiling data. Pyroscope currently supports three major ingestion formats.

### Text formats

These formats handle simple ingestion of profiling data, such as `cpu` samples, and typically don't support metadata (e.g., labels) within the format. All necessary metadata is derived from query parameters, and the format is specified by the `format` query parameter.

**Supported formats:**

- **Folded**: Also known as `collapsed`, this is the default format. Each line contains a stacktrace followed by the sample count for that stacktrace. For example:
```
foo;bar 100
foo;baz 200
```

- **Lines**: Similar to `folded`, but it represents each sample as a separate line rather than aggregating samples per stacktrace. For example:
```
foo;bar
foo;bar
foo;baz
foo;bar
```

### The `pprof` format

The `pprof` format is a widely used binary profiling data format, particularly prevalent in the Go ecosystem.

When using this format, certain query parameters have specific behaviors:

- **format**: This should be set to `pprof`.
- **name**: This parameter contains the _prefix_ of the application name. Since a single request might include multiple profile types, the complete application name is formed by concatenating this prefix with the profile type. For instance, if you send CPU profiling data and set `name` to `my-app{}`, it will be displayed in pyroscope as `my-app.cpu{}`.
- **units**, **aggregationType**, and **sampleRate**: These parameters are ignored. The actual values are determined based on the profile types present in the data (refer to the "Sample Type Configuration" section for more details).

#### Sample type configuration

Pyroscope server inherently supports standard Go profile types such as `cpu`, `inuse_objects`, `inuse_space`, `alloc_objects`, and `alloc_space`. When dealing with software that generates data in `pprof` format, you may need to supply a custom sample type configuration for Pyroscope to interpret the data correctly.

For an example Python script to ingest a `pprof` file with a custom sample type configuration, see **[this Python script](https://github.com/grafana/pyroscope/tree/main/examples/api/ingest_pprof.py).**

To ingest `pprof` data with custom sample type configuration, modify your requests as follows:
* Set Content-Type to `multipart/form-data`.
* Upload the profile data in a form file field named `profile`.
* Include the sample type configuration in a form file field named `sample_type_config`.

A sample type configuration is a JSON object formatted like this:

```json
{
  "inuse_space": {
    "units": "bytes",
    "aggregation": "average",
    "display-name": "inuse_space_bytes",
    "sampled": false
  },
  "alloc_objects": {
    "units": "objects",
    "aggregation": "sum",
    "display-name": "alloc_objects_count",
    "sampled": true
  },
  "cpu": {
    "units": "samples",
    "aggregation": "sum",
    "display-name": "cpu_samples",
    "sampled": true
  },
  // pprof supports multiple profiles types in one file,
  //   so there can be multiple of these objects
}
```

Explanation of sample type configuration fields:

- **units**
  - Supported values: `samples`, `objects`, `bytes`
  - Description: Changes the units displayed in the frontend. `samples` = CPU samples, `objects` = objects in RAM, `bytes` = bytes in RAM.
- **display-name**
  - Supported values: Any string.
  - Description: This becomes a suffix of the app name, e.g., `my-app.inuse_space_bytes`.
- **aggregation**
  - Supported values: `sum`, `average`.
  - Description: Alters how data is aggregated on the frontend. Use `sum` for data to be summed over time (e.g., CPU samples, memory allocations), and `average` for data to be averaged over time (e.g., memory in-use objects).
- **sampled**
  - Supported values: `true`, `false`.
  - Description: Determines if the sample rate (specified in the pprof file) is considered. Set to `true` for sampled events (e.g., CPU samples), and `false` for memory profiles.

This configuration allows for customized visualization and analysis of various profile types within Pyroscope.

### JFR format

This is the [Java Flight Recorder](https://openjdk.java.net/jeps/328) format, typically used by JVM-based profilers, also supported by our Java integration.

When this format is used, some of the query parameters behave slightly different:
* `format` should be set to `jfr`.
* `name` contains the _prefix_ of the application name. Since a single request may contain multiple profile types, the final application name is created concatenating this prefix and the profile type. For example, if you send cpu profiling data and set `name` to `my-app{}`, it will appear in pyroscope as `my-app.cpu{}`.
* `units` is ignored, and the actual units depends on the profile types available in the data.
* `aggregationType` is ignored, and the actual aggregation type depends on the profile types available in the data.

JFR ingestion support uses the profile metadata to determine which profile types are included, which depend on the kind of profiling being done. Currently supported profile types include:
* `cpu` samples, which includes only profiling data from runnable threads.
* `itimer` samples, similar to `cpu` profiling.
* `wall` samples, which includes samples from any threads independently of their state.
* `alloc_in_new_tlab_objects`, which indicates the number of new TLAB objects created.
* `alloc_in_new_tlab_bytes`, which indicates the size in bytes of new TLAB objects created.
* `alloc_outside_tlab_objects`, which indicates the number of new allocated objects outside any TLAB.
* `alloc_in_new_tlab_bytes`, which indicates the size in bytes of new allocated objects outside any TLAB.

#### JFR with labels

In order to ingest JFR data with dynamic labels, you have to make the following changes to your requests:
* use an HTTP form (`multipart/form-data`) Content-Type.
* send the JFR data in a form file field called `jfr`.
* send `LabelsSnapshot` protobuf message in a form file field called `labels`.

```protobuf
message Context {
    // string_id -> string_id
    map<int64, int64> labels = 1;
}
message LabelsSnapshot {
  // context_id -> Context
  map<int64, Context> contexts = 1;
  // string_id -> string
  map<int64, string> strings = 2;
}

```
Where `context_id` is a parameter [set in async-profiler](https://github.com/pyroscope-io/async-profiler/pull/1/files#diff-34c624b2fbf52c68fc3f15dee43a73caec11b9524319c3a581cd84ec3fd2aacfR218)

### Examples

Here's a sample code that uploads a very simple profile to pyroscope:

{{< code >}}

```curl
printf "foo;bar 100\n foo;baz 200" | curl \
-X POST \
--data-binary @- \
'http://localhost:4040/ingest?name=curl-test-app&from=1615709120&until=1615709130'

```

```python
import requests
import urllib.parse
from datetime import datetime

now = round(datetime.now().timestamp()) / 10 * 10
params = {'from': f'{now - 10}', 'name': 'python.example{foo=bar}'}

url = f'http://localhost:4040/ingest?{urllib.parse.urlencode(params)}'
data = "foo;bar 100\n" \
"foo;baz 200"

requests.post(url, data = data)
```

{{< /code >}}


Here's a sample code that uploads a JFR profile with labels to pyroscope:

{{< code >}}

```curl
curl -X POST \
  -F jfr=@profile.jfr \
  -F labels=@labels.pb  \
  "http://localhost:4040/ingest?name=curl-test-app&units=samples&aggregationType=sum&sampleRate=100&from=1655834200&until=1655834210&spyName=javaspy&format=jfr"
```

{{< /code >}}

## Querying profile data

There is one primary endpoint for querying profile data: `GET /pyroscope/render`.

The search input is provided via query parameters.
The output is typically a JSON object containing one or more time series and a flamegraph.

### Query parameters

Here is an overview of the accepted query parameters:

| Name           | Description                                                                           | Notes                                                |
|:---------------|:--------------------------------------------------------------------------------------|:-----------------------------------------------------|
| `query`        | contains the profile type and label selectors                                         | required                                             |
| `from`         | UNIX time for the start of the search window                                          | required                                             |
| `until`        | UNIX time for the end of the search window                                            | optional (default is `now`)                          |
| `format`       | format of the profiling data                                                          | optional (default is `json`)                         |
| `maxNodes`     | the maximum number of nodes the resulting flamegraph will contain                     | optional (default is `max_flamegraph_nodes_default`) |
| `groupBy`      | one or more label names to group the time series by (doesn't apply to the flamegraph) | optional (default is no grouping)                    |

#### `query`

The `query` parameter is the only required search input. It carries the profile type and any labels we want to use to narrow down the output.
The format for this parameter is similar to that of a PromQL query and can be defined as:

`<profile_type>{<label_name>="<label_value>", <label_name>="<label_value>", ...}`

Here is a specific example:

`process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name="my_application_name"}`

In a Kubernetes environment, a query could also look like:

`process_cpu:cpu:nanoseconds:cpu:nanoseconds{namespace="dev", container="my_application_name"}`

{{% admonition type="note" %}}
Refer to the [profiling types documentation]({{< relref "../view-and-analyze-profile-data/profiling-types" >}}) for more information and [profile-metrics.json](https://github.com/grafana/pyroscope/blob/main/public/app/constants/profile-metrics.json) for a list of valid profile types.
{{% /admonition %}}

#### `from` and `until`

The `from` and `until` parameters determine the start and end of the time period for the query.
They can be provided in absolute and relative form.

**Absolute time**

This table details the options for passing absolute values.

| Option                 | Example               | Notes              |
|:-----------------------|:----------------------|:-------------------|
| Date                   | `20231223`            | Format: `YYYYMMDD` |
| Unix Time seconds      | `1577836800`          |                    |
| Unix Time milliseconds | `1577836800000`       |                    |
| Unix Time microseconds | `1577836800000000`    |                    |
| Unix Time nanoseconds  | `1577836800000000000` |                    |

**Relative time**

Relative values are always expressed as offsets from `now`.

| Option         | Example              |
|:---------------|:---------------------|
| 3 hours ago    | `now-3h`             |
| 30 minutes ago | `now-30m`            |
| 2 days ago     | `now-2d`             |
| 1 week ago     | `now-7d` or `now-1w` |

Note that a single offset has to be provided, values such as `now-3h30m` will not work.

**Validation**

The `from` and `until` parameters are subject to validation rules related to `max_query_lookback` and `max_query_length` server parameters.
You can find more details on these parameters in the [limits section]({{< relref "./reference-configuration-parameters#limits" >}}) of the server configuration docs.

- If `max_query_lookback` is configured and`from` is before `now - max_query_lookback`, `from` will be set to `now - max_query_lookback`.
- If `max_query_lookback` is configured and `until` is before `now - max_query_lookback` the query will not be executed.
- If `max_query_length` is configured and the query interval is longer than this configuration, the query will no tbe executed.

#### `format`

The format can either be:
- `json`, in which case the response will contain a JSON object
- `dot`, in which case the response will be text containing a DOT representation of the profile

See the [Query output](#query-output) section for more information on the response structure.

#### `maxNodes`

The `maxNodes` parameter truncates the number of elements in the profile response, to allow tools (for example, a frontend) to render large profiles efficiently.
This is typically used for profiles that are known to have large stack traces.

When no value is provided, the default is taken from the `max_flamegraph_nodes_default` configuration parameter.
When a value is provided, it is capped to the `max_flamegraph_nodes_max` configuration parameter.

#### `groupBy`

The `groupBy` parameter impacts the output for the time series portion of the response.
When a valid label is provided, the response contains as many series as there are label values for the given label.

{{% admonition type="note" %}}
Pyroscope supports a single label for the group by functionality.
{{% /admonition %}}

### Query output

The output of the `/pyroscope/render` endpoint is a JSON object based on the following [schema](https://github.com/grafana/pyroscope/blob/80959aeba2426f3698077fd8d2cd222d25d5a873/pkg/og/structs/flamebearer/flamebearer.go#L28-L43):

```go
type FlamebearerProfileV1 struct {
	Flamebearer FlamebearerV1                  `json:"flamebearer"`
	Metadata FlamebearerMetadataV1             `json:"metadata"`
	Timeline *FlamebearerTimelineV1            `json:"timeline"`
	Groups   map[string]*FlamebearerTimelineV1 `json:"groups"`
}
```

#### `flamebearer`

The `flamebearer` field contains data in a form suitable for rendering a flamegraph.
Data within the flamebearer is organized in separate arrays containing the profile symbols and the sample values.

#### `metadata`

The `metadata` field contains additional information that is helpful to interpret the `flamebearer` data such as the unit (nanoseconds, bytes), sample rate and more.

#### `timeline`

The `timeline` field represents the time series for the profile.
Pyroscope pre-computes the step interval (resolution) of the timeline using the query interval (`from` and `until`). The minimum step interval is 10 seconds.

The raw profile sample data is down-sampled to the step interval (resolution) using an aggregation function. Currently only `sum` is supported.

A timeline contains a start time, a list of sample values and the step interval:

```json
{
  "timeline": {
    "startTime": 1577836800,
    "samples": [
      100,
      200,
      400
    ],
    "durationDelta": 10
  }
}
```

#### `groups`

The `groups` field is only populated when grouping is requested by the `groupBy` query parameter.
When this is the case, the `groups` field has an entry for every label value found for the query.

This example groups by a cluster:

```json
{
  "groups": {
    "eu-west-2": { "startTime": 1577836800, "samples": [ 200, 300, 500 ] },
    "us-east-1": { "startTime": 1577836800, "samples": [ 100, 200, 400 ] }
  }
}
```

### Alternative query output

When the `format` query parameter is `dot`, the endpoint responds with a [DOT format](https://en.wikipedia.org/wiki/DOT_(graph_description_language)) data representing the queried profile.
This can be used to create an alternative visualization of the profile.

### Example queries

This example queries a local Pyroscope server for a CPU profile from the `pyroscope` service for the last hour.

```curl
curl \
  'http://localhost:4040/pyroscope/render?query=process_cpu%3Acpu%3Ananoseconds%3Acpu%3Ananoseconds%7Bservice_name%3D%22pyroscope%22%7D&from=now-1h'
```

Here is the same query made more readable:

```curl
curl --get \
  --data-urlencode "query=process_cpu:cpu:nanoseconds:cpu:nanoseconds{service_name=\"pyroscope\"}" \
  --data-urlencode "from=now-1h" \
  http://localhost:4040/pyroscope/render
```

Here is the same example in Python:

```python
import requests

application_name = 'my_application_name'
query = f'process_cpu:cpu:nanoseconds:cpu:nanoseconds{{service_name="{application_name}"}}'
query_from = 'now-1h'
url = f'http://localhost:4040/pyroscope/render?query={query}&from={query_from}'

requests.get(url)
```

See [this Python script](https://github.com/grafana/pyroscope/tree/main/examples/api/query.py) for a complete example.

## Profile CLI

The `profilecli` tool can also be used to interact with the Pyroscope server API.
The tool supports operations such as ingesting profiles, querying for existing profiles, and more.
Refer to the [Profile CLI]({{< relref "../view-and-analyze-profile-data/profile-cli" >}}) page for more information.
