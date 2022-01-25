## Tracing integration example

The example demonstrates how Pyroscope can be used in conjunction with tracing.
In the example we will instrument a sample application with OpenTelemetry tracer and
will be using [Honeycomb.io](https://www.honeycomb.io) observability platform.

To achieve that, we will be using a special label – `profile_id` that is set by the profiler
dynamically. Our simple application specifies Span ID as a profile ID which establishes
one-to-one relation between a trace span execution scope and the profiling scope. In practice,
the scenario can be different: an arbitrary string can be set as a profile ID. The most important
feature is that profiles with the same ID are merged by storage upon insert.

Pyroscope handles profiles with `profile_id` label in a very specific way and stores them
separately from others. As a result, such profiles can not be accessed using regular queries
that aggregate the data: the very idea of identifiers is to ensure request-level granularity.

There are number of limitations:
 - Only Go CPU profiling is fully supported at the moment.
 - Due to the very idea of the sampling profilers, spans shorter than the sample interval may
   not be captured. For example, Go CPU profiler probes stack traces 100 times per second,
   meaning that spans shorter than 10ms may not be captured.
 - Spans do not account children, the profiler records only the time spent on CPU. This means
   that all the children profiles should be merged to get the full representation of the root
   span profile or profile of the whole trace.

### 1. Run the docker-compose file

 - With debug option: traces aren't sent but printed to stdout.
```
# DEBUG_TRACER=1 docker-compose up --build
```
 - Provide with a valid API Key in order to send traces to Honeycomb. Optionally, you can also set `HONEYCOMB_DATASET` variable. Please see `config.env` for details.
```
#  HONEYCOMB_API_KEY={api_key} docker-compose up --build
```

### 2. Navigate to [Honeycomb UI](https://ui.honeycomb.io)

The newly collected data should be available for querying: Honeycomb allows using various analytical approaches to identify interesting traces.

Notice the `pyroscope.profile.id` span attribute:

![image](https://user-images.githubusercontent.com/12090599/151026887-cb025a2a-6ee5-4401-a3be-73692427f3c1.png)

### 3. Access profiling data via [Pyroscope UI](`http://localhost:4040`)

Now you should be able to access span profiles using its value in Pyroscope UI that is configured to listen on [http://localhost:4040](http://localhost:4040):

![image](https://user-images.githubusercontent.com/12090599/151029404-2e17b817-a133-497a-964b-275f40ca23d6.png)
