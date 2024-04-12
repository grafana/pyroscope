---
title: "NodeJS"
menuTitle: "NodeJS"
description: "Instrumenting nodeJS applications for continuous profiling."
weight: 80
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/nodejs
---

# NodeJS

Enhance your Node.js application's performance with our Node.js Profiler. Seamlessly integrated with Pyroscope, it provides real-time insights into your application’s operation, helping you identify and resolve performance bottlenecks. This integration is key for Node.js developers aiming to boost efficiency, reduce latency, and maintain optimal application performance.

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pryoscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add NodeJS profiling to your application

To start profiling a NodeJS application, you need to include the npm module in your app:

```
npm install @pyroscope/nodejs

# or
yarn add @pyroscope/nodejs
```

## Configure the NodeJS client

Add the following code to your application:

```javascript
const Pyroscope = require('@pyroscope/nodejs');

Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService'
});

Pyroscope.start()
```

Note: If you'd prefer to use Pull mode you can do so using the [Grafana Agent]({{< relref "../grafana-agent" >}}).

### Add profiling labels to NodeJS applications

It is possible to add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. Dynamic tagging isn't supported yet.

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

To configure the NodeJS SDK to send data to Pyroscope, replace the `serverAddress` placeholder with the appropriate server URL. This could be the Grafana Cloud Pyroscope URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you’ll have to configure HTTP Basic authentication. Replace `basicAuthUser` with your Grafana Cloud stack user ID and `basicAuthPassword` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you’ll need to configure a tenant ID. Replace `tenantID` with your Pyroscope tenant ID.

## Troubleshoot

Setting `DEBUG` env to `pyroscope` provides additional debugging information.

```bash
DEBUG=pyroscope node index.js
```
