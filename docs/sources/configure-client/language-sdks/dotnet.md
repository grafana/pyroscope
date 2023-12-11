---
title: ".NET"
menuTitle: ".NET"
description: "Instrumenting .NET applications for continuous profiling"
weight: 40
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/dotnet/
---

# .NET

## Supported platforms

| Spy Name     | Type         | Linux | macOS | Windows | Docker |
|:------------:|:------------:|:-----:|:-----:|:-------:|:------:|
| dotnetspy    | `embedded`   |   ✅  |       |         |        |

## Supported .NET versions:
 - .NET 6.0
 - .NET 7.0

## How to add .NET profiling to your application

1. Obtain `Pyroscope.Profiler.Native.so` and `Pyroscope.Linux.ApiWrapper.x64.so` from the [latest tarball](https://github.com/pyroscope-io/pyroscope-dotnet/releases/):

```bash
curl -s -L https://github.com/grafana/pyroscope-dotnet/releases/download/v0.8.14-pyroscope/pyroscope.0.8.14-glibc-x86_64.tar.gz  | tar xvz -C .
```
Or copy them from the [latest docker image](https://hub.docker.com/r/pyroscope/pyroscope-dotnet/tags):
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


## Managed helper

With a managed helper you can interact with the Pyroscope profiler from .NET.

First, add the Pyroscope dependency:

```shell
dotnet add package Pyroscope
```

## How to add profiling labels to .NET applications

It is possible to add labels to the profiling data. These labels can be used to filter the data in the UI.

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

## Dynamic profiling control

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

It is possible to dynamically change authorization credentials.

```cs
// Set Basic authorization username and password. Clears any previous authorization credentials.
Pyroscope.Profiler.Instance.SetBasicAuth(basicAuthUser, BasicAuthPassword);
```

Here is a simple [example](https://github.com/grafana/pyroscope/blob/main/examples/dotnet/rideshare/example/Program.cs) exposing this APIs as an http endpoint.

## Configuration

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

## Sending data to Pyroscope OSS or Grafana Cloud Profiles with .NET SDK

```bash
export CORECLR_ENABLE_PROFILING=1
export CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
export CORECLR_PROFILER_PATH=/dotnet/Pyroscope.Profiler.Native.so
export LD_PRELOAD=/dotnet/Pyroscope.Linux.ApiWrapper.x64.so
export PYROSCOPE_PROFILING_ENABLED=1
export PYROSCOPE_APPLICATION_NAME=example.dotnet.app
export PYROSCOPE_SERVER_ADDRESS=<URL>
export PYROSCOPE_BASIC_AUTH_USER=<User>
export PYROSCOPE_BASIC_AUTH_PASSWORD=<Password>
# Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
# export PYROSCOPE_TENANT_ID=<TenantID>
```

To configure the .NET SDK to send data to Grafana Cloud Profiles or Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafan Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.
