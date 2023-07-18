---
title: "Java"
menuTitle: "Java"
description: "Instrumenting Java applications for continuous profiling"
weight: 30
---

# Java

## How to add Java profiling to your application

Java integration is distributed as a single jar file: `pyroscope.jar`. It contains native `async-profiler` libraries for:

* Linux on x64;
* Linux on ARM64;
* MacOS on x64.
* MacOS on ARM64.

Visit our GitHub [releases](https://github.com/pyroscope-io/pyroscope-java/releases) page to download the latest version of `pyroscope.jar`.

The latest release is also available on [Maven Central](https://search.maven.org/artifact/io.pyroscope/agent).

## Profiling Java applications

You can start pyroscope either from your apps's java code or attach it as javaagent

## Start pyroscope from app's java code
First, add Pyroscope dependency

### Maven
```xml
<dependency>
  <groupId>io.pyroscope</groupId>
  <artifactId>agent</artifactId>
  <version>pyroscope_version</version>
</dependency>
```

### Gradle
```shell
implementation("io.pyroscope:agent:${pyroscope_version}")
```

Then add the following code to your application:
```java
PyroscopeAgent.start(
  new Config.Builder()
    .setApplicationName("ride-sharing-app-java")
    .setProfilingEvent(EventType.ITIMER)
    .setFormat(Format.JFR)
    .setServerAddress("http://pyroscope-server:4040")
    // Optionally, if authentication is enabled, specify the API key.
    // .setAuthToken(System.getenv("PYROSCOPE_AUTH_TOKEN"))
    .build()
);
```

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

## Start pyroscope as javaagent
To start profiling a Java application, run your application with `pyroscope.jar` javaagent:

```shell
export PYROSCOPE_APPLICATION_NAME=my.java.app
export PYROSCOPE_SERVER_ADDRESS=http://pyroscope-server:4040

# Optionally, if authentication is enabled, specify the API key.
# export PYROSCOPE_AUTH_TOKEN={YOUR_API_KEY}

java -javaagent:pyroscope.jar -jar app.jar
```

## How to add profiling labels to Java applications

It is possible to add dynamic tags (labels) to the profiling data. These tags can be used to filter the data in the UI.

Add labels dynamically:
```java
Pyroscope.LabelsWrapper.run(new LabelsSet("controller", "slow_controller"), () -> {
  slowCode();
});
```

It is also possible to possible to add static tags (labels) to the profiling data:

```java
Pyroscope.setStaticLabels(Map.of("region", System.getenv("REGION")));
// or with Config.Builder if you start pyroscope with PyroscopeAgent.start
PyroscopeAgent.start(new Config.Builder()
    .setLabels(mapOf("region", System.getenv("REGION")))
    // ...
    .build()
);
```

## Java client configuration options

When you start pyroscope as javaagent or obtain configuration by `Config.build()` pyroscope searches 
for configuration in multiple sources: system properties, environment variables, pyroscope.properties file. Properties keys has same name as environment variables, but lowercased and replaced `_` with `.`, so `PYROSCOPE_FORMAT` becomes `pyroscope.format`

Java integration supports JFR format to be able to support multiple events (JFR is the only output format that supports [multiple events in `async-profiler`](https://github.com/jvm-profiling-tools/async-profiler#multiple-events)). There are several environment variables that define how multiple event configuration works:


|Flag                                     |Description                                                                                                                                                                                                                                                                                                                                                                                 |
|-----------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|`PYROSCOPE_FORMAT`                       |sets the profiler output format. The default is `collapsed`, but in order to support multiple formats it must be set to `jfr`.                                                                                                                                                                                                                                                              |
|`PYROSCOPE_PROFILER_EVENT`               |sets the profiler event. With JFR format enabled, this event refers to one of the possible CPU profiling events: `itimer`, `cpu`, `wall`. The default is `itimer`.                                                                                                                                                                                                                          |
|`PYROSCOPE_PROFILER_ALLOC`               |sets the allocation threshold to register the events, in bytes (equivalent to `--alloc=` in `async-profiler`). The default value is "" - empty string, which means that allocation profiling is disabled. Setting it to `0` will register all the events.                                                                                                                                   |
|`PYROSCOPE_PROFILER_LOCK`                |sets the lock threshold to register the events, in nanoseconds (equivalent to `--lock=` in `async-profiler`). The default value is "" - empty string, which means that lock profiling is disabled. Setting it to `0` will register all the events.                                                                                                                                          |
|`PYROSCOPE_CONFIGURATION_FILE`           |sets an additional properties configuration file. The default value is `pyroscope.properties`.                                                                                                                                                                                                                                                                                              |
|`PYROSCOPE_BASIC_AUTH_USER`              |HTTP Basic authentication username. The default value is `""` - empty string, no authentication.                                                                                                                                                                                                                                                                                            |
|`PYROSCOPE_BASIC_AUTH_PASSWORD`          |HTTP Basic authentication password. The default value is `""` - empty string, no authentication.                                                                                                                                                                                                                                                                                            |
|`PYROSCOPE_TENANT_ID`                    |phlare tenant ID, passed as X-Scope-OrgID http header. The default value is `""` - empty string, no tenant ID.                                                                                                                                                                                                                                                                              |
|`PYROSCOPE_HTTP_HEADERS`                 |extra http headers in json format, for example: `{"X-Header": "Value"}`. The default value is `{}` - no extra headers.                                                                                                                                                                                                                                                                      |
|`PYROSCOPE_LABELS`                       |sets static labels in the form of comma separated `key=value` pairs. The default value is `""` - empty string, no labels.                                                                                                                                                                                                                                                                   |
|`PYROSCOPE_LOG_LEVEL`                    |determines the level of verbosity for Pyroscope's logger. Available options include `debug`, `info`, `warn`, and `error`. The default value is set to `info`.                                                                                                                                                                                                                               |
|`PYROSCOPE_PUSH_QUEUE_CAPACITY`          |specifies the size of the ingestion queue that temporarily stores profiling data in memory during network outages. The default value is set to 8.                                                                                                                                                                                                                                           |
|`PYROSCOPE_INGEST_MAX_TRIES`             |sets the maximum number of times to retry an ingestion API call in the event of failure. A value of -1 indicates that the retries will continue indefinitely. The default value is set to 8.                                                                                                                                                                                                |
|`PYROSCOPE_EXPORT_COMPRESSION_LEVEL_JFR` |sets the level of GZIP compression applied to uploaded JFR files. This option accepts values of `NO_COMPRESSION`, `BEST_SPEED`, `BEST_COMPRESSION`, and `DEFAULT_COMPRESSION`.                                                                                                                                                                                                              |
|`PYROSCOPE_EXPORT_COMPRESSION_LEVEL_LABELS`|operates similarly to `PYROSCOPE_EXPORT_COMPRESSION_LEVEL_JFR`, but applies to the dynamic labels part. The default value is set to `BEST_SPEED`.                                                                                                                                                                                                                                           |
|`PYROSCOPE_ALLOC_LIVE`                   |is a boolean value that enables live object profiling when set to true. It is disabled by default.                                                                                                                                                                                                                                                                                          |
|`PYROSCOPE_GC_BEFORE_DUMP`               |is a boolean value that executes a `System.gc()` command before dumping the profile when set to true. This option may be useful for live profiling, but is disabled by default.                                                                                                                                                                                                             |

## Sending data to Grafana Cloud or Phlare with Pyroscope java SDK

To configure java sdk to send data to Phlare, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Phlare server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Phlare server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Phlare tenant ID.

## Java profiling examples

Check out the following resources to learn more about Java profiling:
- [Java examples](https://github.com/grafana/pyroscope/tree/main/examples/java-jfr/rideshare)
- [Java Demo](https://demo.pyroscope.io/?query=rideshare-app-java.itimer%7B%7D) showing Java example with tags
- [Java blog post](https://github.com/grafana/pyroscope/tree/main/examples/java-jfr/rideshare#readme)