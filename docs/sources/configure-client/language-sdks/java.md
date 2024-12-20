---
title: "Java"
menuTitle: "Java"
description: "Instrumenting Java applications for continuous profiling."
weight: 30
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/java
---

# Java

The Java Profiler, integrated with Pyroscope, offers a comprehensive solution for performance analysis in Java applications.
It provides real-time insights, enabling developers to understand and optimize their Java codebase effectively.
This tool is crucial for improving application responsiveness, reducing resource consumption, and ensuring top-notch performance in Java environments.

{{< admonition type="note" >}}
Refer to [Available profiling types](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/profile-types/) for a list of profile types supported by each language.
{{< /admonition >}}

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Java profiling to your application

Java integration is distributed as a single jar file (`pyroscope.jar`) or a Maven package.
Supported platforms include:

* Linux on x64
* Linux on ARM64
* MacOS on x64
* MacOS on ARM64

Visit the GitHub [releases](https://github.com/pyroscope-io/pyroscope-java/releases) page to download the latest version of `pyroscope.jar`.

The latest release is also available on [Maven Central](https://search.maven.org/artifact/io.pyroscope/agent).

## Configure the Java client

You can start Pyroscope either from your application's code or attach it as javaagent.

### Start Pyroscope from app's Java code

First, add the Pyroscope dependency:

{{< code >}}

```maven
<dependency>
  <groupId>io.pyroscope</groupId>
  <artifactId>agent</artifactId>
  <version>0.15.2</version>
</dependency>
```

```gradle
implementation("io.pyroscope:agent:0.15.2")
```

{{< /code >}}

Add the following code to your application:

{{< code >}}

```java
PyroscopeAgent.start(
  new Config.Builder()
    .setApplicationName("ride-sharing-app-java")
    .setProfilingEvent(EventType.ITIMER)
    .setFormat(Format.JFR)
    .setServerAddress("http://pyroscope-server:4040")
    .build()
);
```

```spring
import io.pyroscope.javaagent.PyroscopeAgent;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.javaagent.EventType;
import io.pyroscope.http.Format;

@PostConstruct
public void init() {

    PyroscopeAgent.start(
    new Config.Builder()
        .setApplicationName("ride-sharing-app-java")
        .setProfilingEvent(EventType.ITIMER)
        .setFormat(Format.JFR)
        .setServerAddress("http://pyroscope-server:4040")
        // Optionally, if authentication is enabled, specify the API key.
        // .setBasicAuthUser("<User>")
        // .setBasicAuthPassword("<Password>")
        // Optionally, if you'd like to set allocation threshold to register events, in bytes. '0' registers all events
        // .setProfilingAlloc("0")
        .build()
    );
}
```

{{< /code >}}


You can also optionally replace some Pyroscope components:
```java
PyroscopeAgent.start(
  new PyroscopeAgent.Options.Builder(config)
    .setExporter(snapshot -> {
      // Your custom export/upload logic may go here
      // It is invoked every 10 seconds by default with snapshot of
      // profiling data
    })
    .setLogger((l, msg, args) -> {
      // Your custom logging may go here
      // Pyroscope does not depend on any logging library
      System.out.printf((msg) + "%n", args);
    })
    .setScheduler(profiler -> {
      // Your custom profiling schedule logic may go here
    })
    .build()
);
```

### Start Pyroscope as `javaagent`

To start profiling a Java application, run your application with `pyroscope.jar` `javaagent`:

```shell
export PYROSCOPE_APPLICATION_NAME=my.java.app
export PYROSCOPE_SERVER_ADDRESS=http://pyroscope-server:4040

java -javaagent:pyroscope.jar -jar app.jar
```

### Add profiling labels to Java applications

You can add dynamic tags (labels) to the profiling data. These tags can filter the data in the UI.

Add labels dynamically:
```java
Pyroscope.LabelsWrapper.run(new LabelsSet("controller", "slow_controller"), () -> {
  slowCode();
});
```

You can also add static tags (labels) to the profiling data:

```java
Pyroscope.setStaticLabels(Map.of("region", System.getenv("REGION")));
// or with Config.Builder if you start pyroscope with PyroscopeAgent.start
PyroscopeAgent.start(new Config.Builder()
    .setLabels(mapOf("region", System.getenv("REGION")))
    // ...
    .build()
);
```

### Configuration options

When you start Pyroscope as `javaagent` or obtain configuration by `Config.build()`, Pyroscope searches
for configuration in multiple sources: system properties, environment variables, and `pyroscope.properties`.
Property keys have the same names as environment variables, but are in lowercase and underscores (`_`) are replaced with periods (`.`). For example, `PYROSCOPE_FORMAT` becomes `pyroscope.format`

The Java integration supports JFR format to be able to support multiple events (JFR is the only output format that supports [multiple events in `async-profiler`](https://github.com/jvm-profiling-tools/async-profiler#multiple-events)). There are several environment variables that define how multiple event configuration works:

| Flag                                        | Description                                                                                                                                                                                                                                                                                                                                                                                                                        |
|---------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `PYROSCOPE_AGENT_ENABLED`                   | Enables the agent. The default is `true`.                                                                                                                                                                                                                                                                                                                                                                                          |
| `PYROSCOPE_FORMAT`                          | Sets the profiler output format. The default is `collapsed`, but in order to support multiple formats it must be set to `jfr`.                                                                                                                                                                                                                                                                                                     |
| `PYROSCOPE_PROFILER_EVENT`                  | Sets the profiler event. With JFR format enabled, this event refers to one of the possible CPU profiling events: `itimer`, `cpu`, `wall`. The default is `itimer`.                                                                                                                                                                                                                                                                 |
| `PYROSCOPE_PROFILER_ALLOC`                  | Sets the allocation threshold to register the events, in bytes (equivalent to `--alloc=` in `async-profiler`). The default value is `""` - empty string, which means that allocation profiling is disabled. Setting it to `0` will register every event, causing significant CPU and network overhead, making it not suitable for production environments. We recommend setting a starting value of 512k and adjusting it as needed. |
| `PYROSCOPE_PROFILER_LOCK`                   | Sets the lock threshold to register the events, in nanoseconds (equivalent to `--lock=` in `async-profiler`). The default value is `""` - empty string, which means that lock profiling is disabled. Setting it to `0` will register every event, causing significant CPU and network overhead, making it not suitable for production environments. We recommend setting a starting value of 10ms and adjusting it as needed.        |
| `PYROSCOPE_CONFIGURATION_FILE`              | Sets an additional properties configuration file. The default value is `pyroscope.properties`.                                                                                                                                                                                                                                                                                                                                     |
| `PYROSCOPE_BASIC_AUTH_USER`                 | HTTP Basic authentication username. The default value is `""` - empty string, no authentication.                                                                                                                                                                                                                                                                                                                                   |
| `PYROSCOPE_BASIC_AUTH_PASSWORD`             | HTTP Basic authentication password. The default value is `""` - empty string, no authentication.                                                                                                                                                                                                                                                                                                                                   |
| `PYROSCOPE_TENANT_ID`                       | pyroscope tenant ID, passed as X-Scope-OrgID http header. The default value is `""` - empty string, no tenant ID.                                                                                                                                                                                                                                                                                                                  |
| `PYROSCOPE_HTTP_HEADERS`                    | Extra HTTP headers in JSON format, for example: `{"X-Header": "Value"}`. The default value is `{}` - no extra headers.                                                                                                                                                                                                                                                                                                             |
| `PYROSCOPE_LABELS`                          | Sets static labels in the form of comma separated `key=value` pairs. The default value is `""` - empty string, no labels.                                                                                                                                                                                                                                                                                                          |
| `PYROSCOPE_LOG_LEVEL`                       | Determines the level of verbosity for Pyroscope's logger. Available options include `debug`, `info`, `warn`, and `error`. The default value is set to `info`.                                                                                                                                                                                                                                                                      |
| `PYROSCOPE_PUSH_QUEUE_CAPACITY`             | Specifies the size of the ingestion queue that temporarily stores profiling data in memory during network outages. The default value is set to 8.                                                                                                                                                                                                                                                                                  |
| `PYROSCOPE_INGEST_MAX_TRIES`                | Sets the maximum number of times to retry an ingestion API call in the event of failure. A value of `-1` indicates that the retries will continue indefinitely. The default value is set to `8`.                                                                                                                                                                                                                                       |
| `PYROSCOPE_EXPORT_COMPRESSION_LEVEL_JFR`    | Sets the level of GZIP compression applied to uploaded JFR files. This option accepts values of `NO_COMPRESSION`, `BEST_SPEED`, `BEST_COMPRESSION`, and `DEFAULT_COMPRESSION`.                                                                                                                                                                                                                                                     |
| `PYROSCOPE_EXPORT_COMPRESSION_LEVEL_LABELS` | Operates similarly to `PYROSCOPE_EXPORT_COMPRESSION_LEVEL_JFR`, but applies to the dynamic labels part. The default value is set to `BEST_SPEED`.                                                                                                                                                                                                                                                                                  |
| `PYROSCOPE_GC_BEFORE_DUMP`                  | A boolean value that executes a `System.gc()` command before dumping the profile when set to `true`. This option may be useful for live profiling, but is disabled by default.                                                                                                                                                                                                                                                    |

## Send data to Pyroscope OSS or Grafana Cloud Profiles

Add the following code to your application:
```java
PyroscopeAgent.start(
    new Config.Builder()
        .setApplicationName("test-java-app")
        .setProfilingEvent(EventType.ITIMER)
        .setFormat(Format.JFR)
        .setServerAddress("<URL>")
        // Set these if using Grafana Cloud:
        .setBasicAuthUser("<User>")
        .setBasicAuthPassword("<Password>")
        // Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana cloud.
        // .setTenantID("<TenantID>")
        .build()
);
```

To configure the Java SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

#### Example configurations

The following configuration sets application name, Pyroscope format, profiling interval, event, and lock.
This example is an excerpt from the [`rideshare` Dockerfile](https://github.com/grafana/pyroscope/blob/main/examples/language-sdk-instrumentation/java/rideshare/Dockerfile#L24-L34) available in the Pyroscope repository.

```
ENV PYROSCOPE_APPLICATION_NAME=rideshare.java.push.app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_PROFILER_LOCK=10ms
```

This configuration excerpt enables allocation and lock profiling:

```
PYROSCOPE_PROFILER_ALLOC=512k
PYROSCOPE_PROFILER_LOCK=10ms
```

## Java profiling examples

Check out the following resources to learn more about Java profiling:
- [Java examples](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/java/rideshare)
- [Java Demo](https://play.grafana.org/a/grafana-pyroscope-app/single?query=process_cpu%3Acpu%3Ananoseconds%3Acpu%3Ananoseconds%7Bservice_name%3D%22pyroscope-rideshare-java%22%7D&from=now-1h&until=now) showing Java example with tags
