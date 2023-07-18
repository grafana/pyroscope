# Pyroscope pull mode and Consul Discovery

This example demonstrates how Pyroscope can be used to scrape pprof profiles from services discovered with [Consul](https://developer.hashicorp.com/consul/docs/intro).

The example is a docker compose that consists of:
 - Pyroscope server
 - Single node Consul cluster
 - Demo application that is being profiled

Run the demo:
```shel
docker-compose up --build -d
```

Now that everything is set up, you should be able to browse profiling data via [Pyroscope UI](http://localhost:4040)
and find the demo application called `test-service`.

In the example, our demo application registers itself using Consul API. In practice, the exact service registration
method does not matter: it can be DNS discovery, explicitly defined services, Nomad integration, or any other method.
Please refer to the [Consul documentation](https://developer.hashicorp.com/consul/docs/discovery/services) for details.

```go
// Register the service in consul with the local agent.
config := api.DefaultConfig()
config.Address = consulAddress
client, err := api.NewClient(config)
if err != nil {
	log.Fatalf("unable to initialize consul client: %v", err)
}
hostname, _ := os.Hostname()
registration := &api.AgentServiceRegistration{
	// These parameters are essential as allow both Consul and
	// Pyroscope to reach out to the application.
	Name:    serviceName,
	Port:    servicePort,
	Address: hostname,
	// Service metadata can be propagated to Pyroscope labels,
	// This allows to query profiling data in Pyroscope based on
	// the service metadata attributes.
	Meta: map[string]string{
		"env": "dev",
	},
	Check: &api.AgentServiceCheck{
		HTTP:     fmt.Sprintf("http://%s:%d/health", hostname, servicePort),
		Interval: "10s",
		Timeout:  "30s",
	},
}

if err = client.Agent().ServiceRegister(registration); err != nil {
	log.Fatalf("failed to register consul client: %v", err)
}
```

Pyroscope server is configured to look for profiling targets in the Consul service catalog:

```yaml
---
log-level: debug
scrape-configs:
  - job-name: consul-services
    enabled-profiles: [cpu, mem, goroutines, mutex, block]
    consul-sd-configs:
      - server: 'consul:8500'
        # Optionally specify the datacenter.
        datacenter: "dc1"
        # You may explicitly list services you want to profile.
        # By default, all discovered services are scraped.
        services:
          - test-service

    relabel-configs:
      # Pyroscope server needs application name (the '__name__' label)
      # to be provided for every profile in order to properly aggregate data.
      - source-labels: [__meta_consul_service]
        action: replace
        target-label: __name__

      # Pyroscope server is not aware of the application language and runtime,
      # if profiling data is pulled from the target. You can optionally specify
      # '__spy_name__' label to indicate it: here we take the value from the
      # 'pyroscope_spy_name' metadata item. By default, Go (gospy) is assumed.
      - source-labels:
          [ __meta_consul_service_metadata_pyroscope_spy_name ]
        action: replace
        target-label: __spy_name__
      - regex: __meta_consul_service_metadata_pyroscope_spy_name
        action: labeldrop

      # Labels that are specific to the consul service discovery (__meta_consul_*)
      # can be used as pyroscope labels.
      - source-labels: [__meta_consul_dc]
        action: replace
        target-label: consul_dc

      # Service metadata can be also mapped to pyroscope labels directly.
      - action: labelmap
        regex: __meta_consul_service_metadata_(.+)
```

Note that our service is explicitly listed under `services` parameter, however if you have more sophisticated requirements,
consider using relabeling configuration.

Pyroscope uses the same discovery mechanism as Prometheus does in order to ensure smooth user experience, and it fully
supports [Consul Service Discovery](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#consul_sd_config)
configuration. Note that Pyroscope uses the dash character instead of the underscore character in the configuration option names.
