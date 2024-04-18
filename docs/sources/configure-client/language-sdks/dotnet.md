---
title: ".NET"
menuTitle: ".NET"
description: "Instrumenting .NET applications for continuous profiling."
weight: 40
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/dotnet/
---

# .NET

Our .NET Profiler is a powerful tool designed to enhance the performance analysis and optimization of .NET applications. It seamlessly integrates with Pyroscope, offering real-time insights into the resource usage and bottlenecks within your .NET codebase. This integration empowers developers to pinpoint inefficiencies, improve application speed, and ensure resource-efficient operation.

{{< admonition type="note" >}}
Refer to [Available profiling types]({{< relref "../../view-and-analyze-profile-data/profiling-types#available-profiling-types" >}}) for a list of profile types supported by each language.
{{< /admonition >}}

## Supported profiling types

The .NET Profiler supports the following profiling types:

* CPU
* Wall time
* Allocations
* Lock contention
* Exceptions

### Compatibility

Our .NET profiler works with the following .NET versions:
* .NET 6
* .NET 7
* .NET 8


## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Configure the Dotnet client

1. Obtain `Pyroscope.Profiler.Native.so` and `Pyroscope.Linux.ApiWrapper.x64.so` from the [latest tarball](https://github.com/pyroscope-io/pyroscope-dotnet/releases/):

```bash
curl -s -L https://github.com/grafana/pyroscope-dotnet/releases/download/v0.8.14-pyroscope/pyroscope.0.8.14-glibc-x86_64.tar.gz  | tar xvz -C .
```

Or copy them from the [latest docker image](https://hub.docker.com/r/pyroscope/pyroscope-dotnet/tags). We have `glibc` and `musl` versions:
```dockerfile
COPY --from=pyroscope/pyroscope-dotnet:0.8.14-glibc /Pyroscope.Profiler.Native.so ./Pyroscope.Profiler.Native.so
COPY --from=pyroscope/pyroscope-dotnet:0.8.14-glibc /Pyroscope.Linux.ApiWrapper.x64.so ./Pyroscope.Linux.ApiWrapper.x64.so
````

2. Set the following required environment variables to enable profiler
```shell
PYROSCOPE_APPLICATION_NAME=rideshare.dotnet.app
PYROSCOPE_SERVER_ADDRESS=http://localhost:4040
PYROSCOPE_PROFILING_ENABLED=1
CORECLR_ENABLE_PROFILING=1
CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
CORECLR_PROFILER_PATH=Pyroscope.Profiler.Native.so
LD_PRELOAD=Pyroscope.Linux.ApiWrapper.x64.so
```

&nbsp;

### .NET Profiler API

With a managed helper you can interact with the Pyroscope profiler from .NET runtime. You can add labels, turn on/off profiling types, and more.

To use it, first, add the Pyroscope dependency:

```shell
dotnet add package Pyroscope
```

&nbsp;

### Add profiling labels to your application

You can add labels to the profiling data to filter the data in the UI. Common labels include:
* `hostname`
* `region`
* `team`
* `api_endpoint`

Create a `LabelSet` and wrap a piece of code with `Pyroscope.LabelsWrapper`.

```cs
var labels = Pyroscope.LabelSet.Empty.BuildUpon()
    .Add("key1", "value1")
    .Build();
Pyroscope.LabelsWrapper.Do(labels, () =>
{
  SlowCode();
});
```

Labels can be nested. For nesting LabelSets use `LabelSet.BuildUpon` on non-empty set.
```cs
var labels = Pyroscope.LabelSet.Empty.BuildUpon()
    .Add("key1", "value1")
    .Build();
Pyroscope.LabelsWrapper.Do(labels, () =>
{
  var labels2 = labels.BuildUpon()
    .Add("key2", "value2")
    .Build();
  Pyroscope.LabelsWrapper.Do(labels2, () =>
  {
    SlowCode();
  });
  FastCode();
});
```

### Dynamic control

It is possible to dynamically enable/disable specific profiling types. Profiling types have to be configured prior.

```cs
// Enables or disables CPU/wall profiling dynamically.
// This function works in conjunction with the PYROSCOPE_PROFILING_CPU_ENABLED and
// PYROSCOPE_PROFILING_WALLTIME_ENABLED environment variables. If CPU/wall profiling is not
// configured, this function will have no effect.
Pyroscope.Profiler.Instance.SetCPUTrackingEnabled(enabled);
// Enables or disables allocation profiling dynamically.
// This function works in conjunction with the PYROSCOPE_PROFILING_ALLOCATION_ENABLED environment variable.
// If allocation profiling is not configured, this function will have no effect.
Pyroscope.Profiler.Instance.SetAllocationTrackingEnabled(enabled);
// Enables or disables contention profiling dynamically.
// This function works in conjunction with the PYROSCOPE_PROFILING_LOCK_ENABLED environment variable.
// If contention profiling is not configured, this function will have no effect.
Pyroscope.Profiler.Instance.SetContentionTrackingEnabled(enabled);
// Enables or disables exception profiling dynamically.
// This function works in conjunction with the PYROSCOPE_PROFILING_EXCEPTION_ENABLED environment variable.
// If exception profiling is not configured, this function will have no effect.
Pyroscope.Profiler.Instance.SetExceptionTrackingEnabled(enabled);
```

It is possible to dynamically change authorization credentials:

```cs
// Set Basic authorization username and password. Clears any previous authorization credentials.
Pyroscope.Profiler.Instance.SetBasicAuth(basicAuthUser, BasicAuthPassword);
```

Here is a simple [example](https://github.com/grafana/pyroscope/blob/main/examples/language-sdk-instrumentation/dotnet/rideshare/example/Program.cs) exposing these APIs as HTTP endpoints.

### Configuration options

| ENVIRONMENT VARIABLE                   | Type         | DESCRIPTION                                                                                                                       |
|----------------------------------------|--------------|-----------------------------------------------------------------------------------------------------------------------------------|
| PYROSCOPE_PROFILING_LOG_DIR            | String       | Sets the directory for .NET Profiler logs. Defaults to /var/log/pyroscope/.                                                       |
| PYROSCOPE_LABELS                       | String       | Static labels to apply to an uploaded profile. Must be a list of key:value separated by commas such as: layer:api or team:intake. |
| PYROSCOPE_SERVER_ADDRESS               | String       | Address of the Pyroscope Server                                                                                                   |
| PYROSCOPE_PROFILING_ENABLED            | Boolean      | If set to true, enables the .NET Profiler. Defaults to false.                                                                     |
| PYROSCOPE_PROFILING_WALLTIME_ENABLED   | Boolean      | If set to false, disables the Wall time profiling. Defaults to false.                                                             |
| PYROSCOPE_PROFILING_CPU_ENABLED        | Boolean      | If set to false, disables the CPU profiling. Defaults to true.                                                                    |
| PYROSCOPE_PROFILING_EXCEPTION_ENABLED  | Boolean      | If set to true, enables the Exceptions profiling. Defaults to false.                                                              |
| PYROSCOPE_PROFILING_ALLOCATION_ENABLED | Boolean      | If set to true, enables the Allocations profiling. Defaults to false.                                                             |
| PYROSCOPE_PROFILING_LOCK_ENABLED       | Boolean      | If set to true, enables the Lock Contention profiling. Defaults to false.                                                         |
| PYROSCOPE_BASIC_AUTH_USER              | String       | For HTTP Basic Authentication, use this to send profiles to authenticated server, for example Grafana Cloud                       |
| PYROSCOPE_BASIC_AUTH_PASSWORD          | String       | For HTTP Basic Authentication, use this to send profiles to authenticated server, for example Grafana Cloud                       |
| PYROSCOPE_TENANT_ID                    | String       | Only needed if using multi-tenancy in Pyroscope.                                                                                  |

## Send data to Pyroscope OSS or Grafana Cloud Profiles

To send profiling data from your .NET application, configure your environment for either Pyroscope OSS or Grafana Cloud Profiles using the following steps:

```bash
export CORECLR_ENABLE_PROFILING=1
export CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
export CORECLR_PROFILER_PATH=/dotnet/Pyroscope.Profiler.Native.so
export LD_PRELOAD=/dotnet/Pyroscope.Linux.ApiWrapper.x64.so
export PYROSCOPE_PROFILING_ENABLED=1
export PYROSCOPE_APPLICATION_NAME=example.dotnet.app
export PYROSCOPE_SERVER_ADDRESS=<URL>
# Use these environment variables to send data to Grafana Cloud Profiles
export PYROSCOPE_BASIC_AUTH_USER=<User>
export PYROSCOPE_BASIC_AUTH_PASSWORD=<Password>
# Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
# export PYROSCOPE_TENANT_ID=<TenantID>
```

To configure the .NET SDK to send data to Grafana Cloud Profiles or Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your open source Pyroscope server has multi-tenancy enabled, you'll need to specify a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.
