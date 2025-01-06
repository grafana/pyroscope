package org.example.rideshare;

import io.pyroscope.labels.Pyroscope;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Main {
    public static void main(String[] args) {
        Pyroscope.setStaticLabels(Map.of(
                "region", System.getenv("REGION"),
                "hostname", System.getenv("HOSTNAME")));
        SpringApplication.run(Main.class, args);
    }
}
