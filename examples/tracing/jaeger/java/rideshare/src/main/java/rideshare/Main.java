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
import org.slf4j.LoggerFactory;
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
        Config config = new Config.Builder()
                .setApplicationName(APP_NAME)
                .setProfilingEvent(EventType.ITIMER)
                .setFormat(Format.JFR)
                .setLogLevel(Logger.Level.DEBUG)
                .setServerAddress(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
                .build();
        PyroscopeAgent.start(
                new PyroscopeAgent.Options.Builder(config)
                        .setLogger(new Logger() {
                            org.slf4j.Logger pyroscopeLogger = LoggerFactory.getLogger("pyroscope");

                            @Override
                            public void log(Level l, String msg, Object... args) {
                                String m = String.format(msg, args);
                                switch (l) {
                                    case INFO:
                                        pyroscopeLogger.info(m);
                                        break;
                                    case DEBUG:
                                        pyroscopeLogger.debug(m);
                                        break;
                                    case WARN:
                                        pyroscopeLogger.warn(m);
                                        break;
                                    case ERROR:
                                        pyroscopeLogger.error(m);
                                        break;
                                    default:
                                        break;
                                }
                            }
                        })
                        .build()
        );
        Pyroscope.setStaticLabels(Map.of("region",System.getenv("REGION")));
                }
    }
