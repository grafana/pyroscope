# Pyroscope pull with static targets

This example demonstrates how Pyroscope can be used to scrape pprof profiles from remote nodejs targets.

### 1. Run Pyroscope server and demo application in docker containers

```shell
docker-compose up -d
```

As a sample application we use slightly modified rideshare app

Note that we apply configuration defined in `server.yml`:

<details>
    <summary>server.yml</summary>

```yaml
---
log-level: debug
scrape-configs:
  - job-name: testing
    enabled-profiles: [cpu]
    static-configs:
      - application: rideshare
        spy-name: nodespy
        targets:
          - rideshare:3000
        labels:
          env: dev
```

</details>

### 2. Observe profiling data

Profiling is more fun when the application does some work, so it shipped with built-in load generator.

Now that everything is set up, you can browse profiling data via [Pyroscope UI](http://localhost:4040).
