---
title: "Node.js"
menuTitle: "Node.js"
description: "Instrumenting Node.js applications for continuous profiling."
weight: 80
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/nodejs
---

# Node.js

Enhance your Node.js application's performance with our Node.js Profiler. Seamlessly integrated with Pyroscope, it provides real-time insights into your application’s operation, helping you identify and resolve performance bottlenecks. This integration is key for Node.js developers aiming to boost efficiency, reduce latency, and maintain optimal application performance.

{{< admonition type="note" >}}
Refer to [Available profiling types]({{< relref "../../view-and-analyze-profile-data/profiling-types#available-profiling-types" >}}) for a list of profile types supported by each language.
{{< /admonition >}}

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Node.js profiling to your application

To start profiling a Node.js application, you need to include the npm module in your app:

```
npm install @pyroscope/nodejs

# or
yarn add @pyroscope/nodejs
```

## Configure the Node.js client

Add the following code to your application:

```javascript
const Pyroscope = require('@pyroscope/nodejs');

Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService'
});

Pyroscope.start()
```

[comment]: <> (TODO This needs its own page like https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/go_pull/)
{{< admonition type="note" >}}
If you'd prefer, you can use Pull mode using [Grafana Alloy](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/go_pull/).
{{< /admonition >}}


### Configuration options

| Init parameter                | ENVIRONMENT VARIABLE                      | Type           | DESCRIPTION                                                                       |
|-------------------------------|-------------------------------------------|----------------|-----------------------------------------------------------------------------------|
| `appName:                     | `PYROSCOPE_APPLICATION_NAME`              | String         | Sets the `service_name` label                                                     |
| `serverAddress:`              | `PYROSCOPE_SERVER_ADDRESS`                | String         | URL of the Pyroscope Server                                                       |
| `basicAuthUser:`              | n/a                                       | String         | Username for basic auth / Grafana Cloud stack user ID (Default `""`)              |
| `basicAuthPassword:`          | n/a                                       | String         | Password for basic auth / Grafana Cloud API key (Default `""`)                    |
| `flushIntervalMs:`            | `PYROSCOPE_FLUSH_INTERVAL_MS`             | Number         | Interval when profiles are sent to the server (Default `60000`)                   |
| `heapSamplingIntervalBytes`   | `PYROSCOPE_HEAP_SAMPLING_INTERVAL_BYTES`  | Number         | Average number of bytes between samples. (Default `524288`)                       |
| `heapStackDepth:`             | `PYROSCOPE_HEAP_STACK_DEPTH`              | Number         | Maximum stack depth for heap samples (Default `64`)                               |
| `wallSamplingDurationMs:`     | `PYROSCOPE_WALL_SAMPLING_DURATION_MS`     | Number         | Duration of a single wall profile (Default `60000`)                               |
| `wallSamplingIntervalMicros:` | `PYROSCOPE_WALL_SAMPLING_INTERVAL_MICROS` | Number         | Interval of how often wall samples are collected (Default `10000`                 |
| `wallCollectCpuTime:`         | `PYROSCOPE_WALL_COLLECT_CPU_TIME`         | Boolean        | Enable CPU time collection for wall profiles (Default `false`)                    |
| `tags:`                       | n/a                                       | [LabelSet]     | Static labels applying to all profiles collected (Default `{}`)                   |
| `sourceMapper:`               | n/a                                       | [SourceMapper] | Provide source file mapping information (Default `undefined`)                     |

[LabelSet]:https://github.com/DataDog/pprof-nodejs/blob/v5.3.0/ts/src/v8-types.ts#L59-L61
[SourceMapper]:https://github.com/DataDog/pprof-nodejs/blob/v5.3.0/ts/src/sourcemapper/sourcemapper.ts#L152


### Add profiling labels to Node.js applications

#### Static labels

You can add static labels to the profiling data.
These labels can be used to filter the data in the UI and apply for all profiles collected.
Common static labels include:

* `hostname`
* `region`
* `team`

```javascript
Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService',
  tags: {
    region: ENV['region']
  },
});

Pyroscope.start()
```

#### Dynamic labels for Wall/CPU profiles

In Wall and CPU profiles, labels can also be attached dynamically and help to separate different code paths:

```javascript
Pyroscope.wrapWithLabels({ vehicle: 'bike' }, () =>
  slowCode()
);
```

## Send data to Pyroscope OSS or Grafana Cloud

```javascript
Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService',
  tags: {
    region: ENV['region']
  },
  basicAuthUser: ENV['PYROSCOPE_BASIC_AUTH_USER'],
  basicAuthPassword: ENV['PYROSCOPE_BASIC_AUTH_PASSWORD'],
  // Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
  // tenantID: ENV['PYROSCOPE_TENANT_ID'],
});

Pyroscope.start()
```

To configure the Node.js SDK to send data to Pyroscope, replace the `serverAddress` placeholder with the appropriate server URL. This could be the Grafana Cloud Pyroscope URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you’ll have to configure HTTP Basic authentication. Replace `basicAuthUser` with your Grafana Cloud stack user ID and `basicAuthPassword` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you’ll need to configure a tenant ID. Replace `tenantID` with your Pyroscope tenant ID.

## Troubleshoot

Setting `DEBUG` env to `pyroscope` provides additional debugging information.

```bash
DEBUG=pyroscope node index.js
```
