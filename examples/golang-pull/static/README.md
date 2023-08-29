# Pyroscope pull with static targets

This example demonstrates how Pyroscope can be used together with Grafana Agent to scrape pprof profiles from remote static targets.

### 1. Run Pyroscope server and demo application in docker containers

```shell
docker-compose up -d
```

As a sample application we use slightly modified Jaeger [Hot R.O.D.](https://github.com/jaegertracing/jaeger/tree/master/examples/hotrod) demo –
the only difference is that we enabled built-in Go `pprof` HTTP endpoints. You can find the modified code in the [hotrod-goland](https://github.com/pyroscope-io/hotrod-golang) repository.

Note that we apply configuration defined in Grafana Agent `config.river`:

<details>
    <summary>config.river</summary>

```
logging {
  level = "debug"
  format = "logfmt"
}

pyroscope.write "example" {
  // Send metrics to a locally running Pyroscope instance.
  endpoint {
    url = "http://pyroscope:4040"

    // To send data to Grafana Cloud you'll need to provide username and password.
    // basic_auth {
    //   username = "myuser"
    //   password = "mypassword"
    // }
  }
  external_labels = {
    "env" = "example",
  }
}


pyroscope.scrape "default" {
  targets = [
    {"__address__" = "hotrod:6060", "service_name"="hotrod"},
    {"__address__" = "app:6060", "service_name"="app"},
  ]
  forward_to = [pyroscope.write.example.receiver]
}
```

</details>

### 2. Observe profiling data

Profiling is more fun when the application does some work. Let's order some rides [in our Hot R.O.D. app](http://localhost:8080).

Now that everything is set up, you can browse profiling data via [Pyroscope UI](http://localhost:4040).
