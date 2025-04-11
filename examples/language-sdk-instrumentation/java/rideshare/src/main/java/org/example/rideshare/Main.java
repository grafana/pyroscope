package org.example.rideshare;

import io.pyroscope.javaagent.PyroscopeAgent;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.javaagent.impl.DefaultConfigurationProvider;
import io.pyroscope.labels.v2.Pyroscope;
import org.jetbrains.annotations.NotNull;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Main {
    public static void main(String[] args) {
        Pyroscope.setStaticLabels(Map.of(
                "region", env("REGION", "us-east-1"),
                "hostname", env("HOSTNAME", "localhost")));
        if (!PyroscopeAgent.isStarted()) {
            // If we have not started the sdk with -javaagent (for example running from an IDE)
            // allow starting the sdk here for convenience
            PyroscopeAgent.start(Config.build(DefaultConfigurationProvider.INSTANCE));
        }
        SpringApplication.run(Main.class, args);
    }

    public static @NotNull String env(@NotNull String key, @NotNull String fallback) {
        final String env = System.getenv(key);
        if (env == null) {
            return fallback;
        }
        return env;
    }
}
