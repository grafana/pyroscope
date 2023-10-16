---
description: Learn about the Pyrocope server API
menuTitle: Server API Overview
title: Pyroscope Server HTTP API Reference
weight: 20
---

# Pyroscope Server HTTP API Reference

Pyroscope server exposes an HTTP API that can be used to query profiling data and to ingest profiling data from other sources.

## Authentication
TODO - how does authentication work with new Grafana cloud stuff?

## Ingestion

Currently there's just one endpoint: `POST /ingest`. It takes profile data in request body and metadata as query params.

It takes multiple query parameters:

| Name               | Description                             | Notes                           |
|:-------------------|:----------------------------------------|:--------------------------------|
| `name`             | application name                        | required |
| `from`             | UNIX time of when the profiling started | required                        |
| `until`            | UNIX time of when the profiling stopped | required                        |
| `format`           | format of the profiling data            | optional (default is `folded`)  |
| `sampleRate`       | sample rate used in Hz                  | optional (default is 100 Hz)    |
| `spyName`          | name of the spy used                    | optional                        |
| `units`            | name of the profiling data unit         | optional (default is `samples`  |
| `aggregrationType` | type of aggregration to merge profiles  | optional (default is `sum`)     |

* `name` specifies application name. For example:
```
my.awesome.app.cpu{env=staging,region=us-west-1}
```

Request body contains the profiling data, and request header `Content-Type` may also be alongside `format` to determine profiling data format.

Some of the query parameters depend on the format of profiling data. Pyroscope currently supports three major ingestion formats.

### Text Formats

Most simple ingestion formats send a single type of profiling data (for example, `cpu` samples) and don't usually support any metadata (e.g labels) within the format to indicate which kind of data they are sending.
In these cases, all the metadata is taken from the query parameters, and the format itself is defined by `format` query parameter.
The formats that work this way are:

* `folded`. Some software also call this format `collapsed`. This is the default format. With this format you put one stacktrace per line with a number of samples you've captured for that particular stacktrace, for example:
```
foo;bar 100
foo;baz 200
```
* `lines` — This format is similar to `folded`, except there's no number of samples per stacktrace but a single line per sample, for example:
```
foo;bar
foo;bar
foo;baz
foo;bar
```

### pprof format

`pprof` is a binary profiling data format popular in many languages, especially in the Go ecosystem.

When this format is used, some of the query parameters behave slightly different:
* `format` should be set to `pprof`.
* `name` contains the _prefix_ of the application name. Since a single request may contain multiple profile types, the final application name is created concatenating this prefix and the profile type. For example, if you send cpu profiling data and set `name` to `my-app{}`, it will appear in pyroscope as `my-app.cpu{}`.
* `units`, `aggregationType` and `sampleRate` are ignored, and the actual values depends on the profile types available in the data (see the next section, "Sample Type Configuration").

#### Sample Type Configuration

Out of the box Pyroscope server supports [default Go profile types](https://github.com/pyroscope-io/pyroscope/blob/main/pkg/storage/tree/pprof.go#L37-L75) (`cpu`, `inuse_objects`, `inuse_space`, `alloc_objects`, `alloc_space`). When working with other software that generates data in pprof format you may have to provide a custom sample type configuration for pyroscope to be able to parse the data properly.

In order to ingest pprof data with a custom sample type configuration, you have to make the following changes to your requests:
* use an HTTP form (`multipart/form-data`) Content-Type.
* send the profile data in a form file field called `profile`.
* send the sample type configuration in a form file field called `sample_type_config`.

Sample type configuration is a JSON object that looks like this:

```json
{
  "inuse_space": { // "inuse_space" here is the internal pprof type name
    "units": "bytes",
    "aggregation": "average",
    "display-name": "inuse_space_bytes",
    "sampled": false,
  },
  // pprof supports multiple profiles types in one file,
  //   so there can be multiple of these objects
}
```


Here's a description of sample type configuration fields:

* `units`
  * Supported values: `samples`, `objects`, `bytes`
  * Description: This will change the units shown on the frontend. `samples` = cpu samples, `objects` = objects in RAM, `bytes` = bytes in RAM
* `display-name`
  * Supported values: any string
  * Description: this becomes a suffix of app name, for example `my-app.inuse_space_bytes`
* `aggregation`
  * Supported values: `sum`, `average`
  * Description: This changes how data is aggregated when shown on the frontend. use `sum` for data that you want to sum over time (e.g cpu samples, memory allocations), use `average` for data that you want to average over time (e.g. memory memory in-use objects)
* `sampled`
  * Supported values: `true` / `false`
  * Description: This parameter defines if sample rate (specified in pprof file) is going to be taken into account or not. Set this to `true` for sampled events (e.g CPU samples), set it to `false` for memory profiles.


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
