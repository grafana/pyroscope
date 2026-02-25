package org.example.rideshare;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.exporter.otlp.grpc.OtlpGrpcSpanExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.resources.Resource;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import io.opentelemetry.sdk.trace.export.BatchSpanProcessor;
import io.opentelemetry.semconv.ServiceAttributes;
import io.otel.pyroscope.PyroscopeOtelConfiguration;
import io.otel.pyroscope.PyroscopeOtelSpanProcessor;
import io.pyroscope.http.Format;
import io.pyroscope.javaagent.EventType;
import io.pyroscope.javaagent.PyroscopeAgent;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.labels.v2.Pyroscope;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.Map;

@SpringBootApplication
public class Main {
    public static void main(String[] args) {
        Pyroscope.setStaticLabels(Map.of(
                "region", System.getenv("REGION"),
                "hostname", System.getenv("HOSTNAME")));

        PyroscopeAgent.start(
                new Config.Builder()
                        .setApplicationName("rideshare.java.push.app")
                        .setProfilingEvent(EventType.ITIMER)
                        .setFormat(Format.JFR)
                        .setProfilingLock("10ms")
                        .setProfilingAlloc("512k")
                        .setServerAddress(System.getenv().getOrDefault(
                                "PYROSCOPE_SERVER_ADDRESS", "http://localhost:4040"))
                        .build()
        );

        SpringApplication.run(Main.class, args);
    }

    @Bean
    public OpenTelemetry openTelemetry() {
        String otlpEndpoint = System.getenv().getOrDefault(
                "OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317");
        String serviceName = System.getenv().getOrDefault(
                "OTEL_SERVICE_NAME", "rideshare.java.push.app");

        Resource resource = Resource.getDefault().merge(
                Resource.builder()
                        .put(ServiceAttributes.SERVICE_NAME, serviceName)
                        .build());

        OtlpGrpcSpanExporter exporter = OtlpGrpcSpanExporter.builder()
                .setEndpoint(otlpEndpoint)
                .build();

        SdkTracerProvider tracerProvider = SdkTracerProvider.builder()
                .setResource(resource)
                .addSpanProcessor(BatchSpanProcessor.builder(exporter).build())
                .addSpanProcessor(new PyroscopeOtelSpanProcessor(
                        new PyroscopeOtelConfiguration.Builder().build(), null))
                .addSpanProcessor(new RootSpanPrinterProcessor())
                .build();

        return OpenTelemetrySdk.builder()
                .setTracerProvider(tracerProvider)
                .buildAndRegisterGlobal();
    }
}
