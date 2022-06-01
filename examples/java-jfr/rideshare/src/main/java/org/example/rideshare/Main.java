package org.example.rideshare;

import io.pyroscope.labels.Labels;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Main {
    public static void main(String[] args) {
        Labels.setStaticLabels(Map.of("REGION", System.getenv("REGION")));
        SpringApplication.run(Main.class, args);
    }
}
