---
title: "NodeJS"
menuTitle: "NodeJS"
description: "Instrumenting nodeJS applications for continuous profiling"
weight: 80
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/nodejs
---

# NodeJS

## How to add NodeJS profiling to your application

To start profiling a NodeJS application, you need to include the npm module in your app:

```
npm install @pyroscope/nodejs

# or
yarn add @pyroscope/nodejs
```

Then add the following code to your application:

```javascript
const Pyroscope = require('@pyroscope/nodejs');

Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService'
});

Pyroscope.start()
```

## How to add profiling labels to NodeJS applications

It is possible to add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. Dynamic tagging isn't supported yet.

```javascript
Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService',
  tags: {
    region: ENV['region']
  },
  // authToken: ENV['PYROSCOPE_AUTH_TOKEN'],
  // basicAuthUser: ENV['PYROSCOPE_BASIC_AUTH_USER'],
  // basicAuthPassword: ENV['PYROSCOPE_BASIC_AUTH_PASSWORD'],
  // tenantID: ENV['PYROSCOPE_TENANT_ID'],
});

Pyroscope.start()
```

## Pull mode profiling for NodeJS

The NodeJS integration also supports pull mode. For that to work you will need to make sure you have profiling routes (`/debug/pprof/profile` and `/debug/pprof/heap`) enabled in your http server. You may use our `expressMiddleware` or create endpoints yourself.

```javascript
const Pyroscope, { expressMiddleware } = require('@pyroscope/nodejs');

Pyroscope.init({...})

app.use(expressMiddleware())
```

Note: For __pull mode__, you don't need to `.start()` but you'll need to `.init()`.

### Scrape configuration

You will need to add the following content to your `config.yml` Pyroscope config file. See the [Server config documentation]({{< relref "../../configure-server/reference-configuration-parameters/" >}}) for more information on where this config is located by default on your system.

```yaml
---
# A list of scrape configurations.
scrape-configs:
  # The job name assigned to scraped profiles by default.
  - job-name: pyroscope

    # The list of profiles to be scraped from the targets.
    enabled-profiles: [cpu, mem]

    # List of labeled statically configured targets for this job.
    static-configs:
      - application: my-nodejsapp-name
        spy-name: nodespy
        targets:
          - hostname:6060
        labels:
          env: dev
```

## Sending data to Pyroscope OSS or Grafana Cloud with Pyroscope NodeJS SDK

To configure the NodeJS SDK to send data to Pyroscope, replace the `serverAddress` placeholder with the appropriate server URL. This could be the Grafana Cloud Pyroscope URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you’ll have to configure HTTP Basic authentication. Replace `basicAuthUser` with your Grafana Cloud stack user ID and `basicAuthPassword` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you’ll need to configure a tenant ID. Replace `tenantID` with your Pyroscope tenant ID.

## Troubleshooting

Setting `DEBUG` env to `pyroscope` will provide additionally debugging information.

```bash
DEBUG=pyroscope node index.js
```
