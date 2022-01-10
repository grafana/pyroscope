# Pyroscope pull with static targets

This example demonstrates how Pyroscope can be used to scrape pprof profiles from remote static targets.

### 1. Run Pyroscope server and demo application in docker containers

```shell
docker-compose up -d
```

As a sample application we use slightly modified Jaeger [Hot R.O.D.](https://github.com/jaegertracing/jaeger/tree/master/examples/hotrod) demo –
the only difference is that we enabled built-in Go `pprof` HTTP endpoints. You can find the modified code in the [hotrod-goland](https://github.com/pyroscope-io/hotrod-golang) repository.

Note that we apply configuration defined in `server.yml`:

<details>
    <summary>server.yml</summary>

```yaml
---
log-level: debug
scrape-configs:
  - job-name: testing
    enabled-profiles: [cpu, mem]
    static-configs:
      - application: hotrod
        targets:
          - hotrod:6060
        labels:
          env: dev
```

</details>

### 2. Observe profiling data

Profiling is more fun when the application does some work. Let's order some rides [in our Hot R.O.D. app](http://localhost:8080).

Now that everything is set up, you can browse profiling data via [Pyroscope UI](http://localhost:4040).
