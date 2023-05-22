---
title: "NodeJS"
menuTitle: "NodeJS"
description: "Instrumenting nodeJS applications for continuous profiling"
weight: 30
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

It is possible to add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. Dynamic tagging isn't supported yet

```javascript
Pyroscope.init({
  serverAddress: 'http://pyroscope:4040',
  appName: 'myNodeService',
  tags: {
    region: ENV['region']
  }
});

Pyroscope.start()
```

## Pull Mode profiling for NodeJS

NodeJS integration also supports pull mode. For that to work you will need to make sure you have profiling routes (`/debug/pprof/profile` and `/debug/pprof/heap`) enabled in your http server. For that you may use our `expressMiddleware` or create endpoints yourself
```javascript
const Pyroscope, { expressMiddleware } = require('@pyroscope/nodejs');

Pyroscope.init({...})

app.use(expressMiddleware())
```

Note: you don't need to `.start()` but you'll need to `.init()`

### Scrape configuration

You will need to add the following content to your `pyroscope/server.yml` Pyroscope config file. See the [Server config documentation](/docs/server-configuration#configuration-file) for more information on where this config is located by default on your system.

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


## Troubleshooting

You may set `DEBUG` env to `pyroscope` and see debugging information which can help you understand if everything is OK.

```bash
DEBUG=pyroscope node index.js
```

