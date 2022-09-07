package rideshare;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.trace.propagation.W3CTraceContextPropagator;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.exporter.jaeger.JaegerGrpcSpanExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.resources.Resource;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import io.opentelemetry.sdk.trace.SpanProcessor;
import io.opentelemetry.sdk.trace.data.LinkData;
import io.opentelemetry.sdk.trace.export.BatchSpanProcessor;
import io.opentelemetry.sdk.trace.export.SpanExporter;
import io.opentelemetry.sdk.trace.samplers.Sampler;
import io.opentelemetry.sdk.trace.samplers.SamplingResult;
import io.opentelemetry.semconv.resource.attributes.ResourceAttributes;
import io.otel.pyroscope.PyroscopeOtelConfiguration;
import io.otel.pyroscope.PyroscopeOtelSpanProcessor;
import io.otel.pyroscope.shadow.http.Format;
import io.otel.pyroscope.shadow.javaagent.EventType;
import io.otel.pyroscope.shadow.javaagent.PyroscopeAgent;
import io.otel.pyroscope.shadow.javaagent.api.Logger;
import io.otel.pyroscope.shadow.javaagent.config.Config;
import io.otel.pyroscope.shadow.labels.Pyroscope;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.cloud.sleuth.autoconfig.otel.OtelProperties;
import org.springframework.cloud.sleuth.autoconfig.otel.SpanProcessorProvider;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.List;
import java.util.Map;

import static rideshare.Main.APP_NAME;

@Configuration
public class OtelConfig {

    @Bean
    ContextPropagators otelContextPropagators() {
        return ContextPropagators.create(W3CTraceContextPropagator.getInstance());
    }

    @Bean
    Config pyroscopeConfig() {
        return new Config.Builder()
                .setApplicationName(APP_NAME)
                .setProfilingEvent(EventType.ITIMER)
                .setFormat(Format.JFR)
                .setLogLevel(Logger.Level.DEBUG)
                .setServerAddress(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
                .build();
    }

    @Bean
    SpanProcessor pyroscopeSpanProcessor(Config pyroscopeConfig) {
        Pyroscope.setStaticLabels(Map.of("region", System.getenv("REGION")));
        PyroscopeAgent.start(
                new PyroscopeAgent.Options.Builder(pyroscopeConfig)
                        .setLogger(new PyroscopeLog4jLogger())
                        .build()
        );

        PyroscopeOtelConfiguration pyroscopeOtelConfig = new PyroscopeOtelConfiguration.Builder()
                .setAppName(pyroscopeConfig.applicationName + "." + pyroscopeConfig.profilingEvent.id)
                .setPyroscopeEndpoint(System.getenv("PYROSCOPE_PROFILE_URL"))
                .setAddProfileURL(true)
                .setAddSpanName(true)
                .setAddProfileBaselineURLs(true)
                .build();
        return new PyroscopeOtelSpanProcessor(pyroscopeOtelConfig);
    }

    @Bean
    Sampler otelSampler(OtelProperties otelProperties) {
        return Sampler.alwaysOn();
    }

    @Bean
    SpanExporter exporter() {
        JaegerGrpcSpanExporter exporter =
                JaegerGrpcSpanExporter.builder()
                        .setEndpoint(System.getenv("OTEL_EXPORTER_JAEGER_ENDPOINT"))
                        .build();
        return exporter;
    }

    @Bean
    public Tracer tracer(OpenTelemetry telemetry) {
        return telemetry.getTracer(APP_NAME);
    }

    @Bean
    Resource otelResource() {
        return Resource.getDefault()
                .merge(Resource.create(Attributes.of(ResourceAttributes.SERVICE_NAME, APP_NAME)));
    }

}
