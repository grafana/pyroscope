---
title: "JVM"
menuTitle: "JVM"
description: ""
weight: 20
---

# JVM

JVM and Java don't support natively [pprof endpoints](https://pkg.go.dev/net/http/pprof), but we are building
a library ([jpprof](https://github.com/grafana/JPProf)) to make it possible.

The library currently is available on the [maven central repository](https://search.maven.org/search?q=g:com.grafana) under the `com.grafana` group.
It currently only supports CPU endpoints, but we are working on adding more.

## Adding the dependency

If you're using maven, add the following dependency to your `pom.xml`:

```xml
<dependency>
  <groupId>com.grafana</groupId>
  <artifactId>jpprof</artifactId>
  <version>0.1.0</version>
</dependency>
```

If you're using gradle, add the following dependency to your `build.gradle`:

```groovy
dependencies {
// ....
    implementation 'com.grafana:jpprof:0.1.0'
//...
}
```

## Integrating with Spring Boot

If you're using Spring Boot, you can add the following [Controller](https://docs.spring.io/spring-framework/docs/current/javadoc-api/org/springframework/web/bind/annotation/RestController.html) to your application:

```java
package com.example.springboot;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseBody;

import javax.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.time.Duration;

import jpprof.CPUProfiler;

@RestController
public class PprofController {

  @GetMapping("/debug/pprof/profile")
  @ResponseBody
  public void profile(@RequestParam(required = false) String seconds, HttpServletResponse response) {
    try {
      Duration d = Duration.ofSeconds(Integer.parseInt(seconds));
      CPUProfiler.start(d, response.getOutputStream());
      response.flushBuffer();
    } catch (Exception e) {
      System.out.println("exception: " + e.getMessage());
    }
  }
}
```

## Integrating with a standalone server

If you're using another framework than Spring Boot, you can still use the library by adding the following lines to your code:

```java
package com.example;

import java.net.InetSocketAddress;
import com.sun.net.httpserver.HttpServer;
import jpprof.PprofHttpHandler;

public class Main {

    public static void main(String[] args) throws Exception {
        //.... your main code ...
        var server = HttpServer.create(new InetSocketAddress(8080), 0);
        server.createContext("/", new PprofHttpHandler());
        server.start();
    }

}
```

This will expose the pprof endpoints on the root path using a new server listening on port `8080`.

## Docker Considerations

[jpprof](https://github.com/grafana/JPProf) uses behind the scenes [async-profiler](https://github.com/jvm-profiling-tools/async-profiler) which takes
advantage of [perf_event_open](https://perf.wiki.kernel.org/index.php/Tutorial#perf_event_open) to collect the CPU samples.

This means that you need to run your container with the `SYS_ADMIN` capability and/or the `--privileged` flag.

> async-profiler also requires JVM symbols to be available, use an appropriate docker base image for your JVM version.

Make sure to checkout our [docker-compose example](https://github.com/grafana/phlare/tree/main/tools/docker-compose).


## Scrape Target Configuration

Because the support is currently only for CPU endpoints, you need to add the following configuration to your scrape target:

```yaml
  - job_name: "java"
    scrape_interval: "15s"
    static_configs:
      - targets: ["my-java-app:8080"]
    profiling_config:
      pprof_config:
        block: { enabled: false }
        goroutine: { enabled: false }
        memory: { enabled: false }
        mutex: { enabled: false }
```

This way, the agent will only scrape the CPU endpoint.
