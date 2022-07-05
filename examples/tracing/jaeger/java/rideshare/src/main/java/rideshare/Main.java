package rideshare;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.trace.propagation.W3CTraceContextPropagator;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.exporter.jaeger.JaegerGrpcSpanExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import io.opentelemetry.sdk.trace.export.BatchSpanProcessor;
import io.pyroscope.http.Format;
import io.pyroscope.javaagent.EventType;
import io.pyroscope.javaagent.PyroscopeAgent;
import io.pyroscope.javaagent.api.Logger;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.labels.Pyroscope;
import io.pyroscope.otel.PyroscopeTelemetry;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.Map;

@SpringBootApplication
public class Main {

    public static final String APP_NAME = "ride-sharing-app-java";

    public static void main(String[] args) {
        configurePyroscope();
        SpringApplication.run(Main.class, args);
    }

    private static void configurePyroscope() {
        PyroscopeAgent.start(
                new Config.Builder()
                        .setApplicationName(APP_NAME)
                        .setProfilingEvent(EventType.ITIMER)
                        .setFormat(Format.JFR)
                        .setLogLevel(Logger.Level.DEBUG)
                        .setServerAddress(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
                        .build()
        );
        Pyroscope.setStaticLabels(Map.of("region", System.getenv("REGION")));
    }
}
