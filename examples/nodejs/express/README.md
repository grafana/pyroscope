# Pyroscope push mode

This example demonstrates how Pyroscope can be used to profile nodejs server.

### 1. Run Pyroscope server and demo application in docker containers

```shell
docker-compose up -d
```

As a sample application we use slightly modified rideshare app

Note: you may want to configure pyroscope server by provisioning an env var

```
export PYROSCOPE_SERVER=http://localhost:4040
node index.js
```

### 2. Observe profiling data

Profiling is more fun when the application does some work, so it shipped with built-in load generator.

Now that everything is set up, you can browse profiling data via [Pyroscope UI](http://localhost:4040).
