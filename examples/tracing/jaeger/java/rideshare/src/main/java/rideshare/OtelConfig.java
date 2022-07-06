package rideshare;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.trace.propagation.W3CTraceContextPropagator;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.exporter.jaeger.JaegerGrpcSpanExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.resources.Resource;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import io.opentelemetry.sdk.trace.export.BatchSpanProcessor;
import io.opentelemetry.semconv.resource.attributes.ResourceAttributes;
import io.pyroscope.javaagent.EventType;
import io.pyroscope.otel.PyroscopeTelemetry;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import static rideshare.Main.APP_NAME;

@Configuration
public class OtelConfig {

    @Bean
    ContextPropagators otelContextPropagators() {
        return ContextPropagators.create(W3CTraceContextPropagator.getInstance());
    }

    @Bean
    JaegerGrpcSpanExporter exporter() {
        JaegerGrpcSpanExporter exporter =
                JaegerGrpcSpanExporter.builder()
                        .setEndpoint(System.getenv("JAEGER_GRPC_ENDPOINT"))
                        .build();
        return exporter;
    }

    @Bean
    public OpenTelemetry telemetry(
            ContextPropagators contextPropagators,
            JaegerGrpcSpanExporter exporter,
            Resource resource
    ) {
        PyroscopeTelemetry.Config pyroscopeTelemetryConfig = new PyroscopeTelemetry.Config.Builder()
                .setAppName(APP_NAME + "." + EventType.ITIMER.id)
                .setPyroscopeEndpoint(System.getenv("PYROSCOPE_PROFILE_URL"))
                .setAddProfileURL(true)
                .setAddSpanName(true)
                .setAddProfileBaselineURLs(true)
                .build();


        final SdkTracerProvider tracerProvider =
                SdkTracerProvider.builder()
                        .setResource(resource)
                        .addSpanProcessor(
                                BatchSpanProcessor.builder(exporter).build()
                        )
                        .build();


        OpenTelemetrySdk sdkTelemetry = OpenTelemetrySdk.builder()
                .setPropagators(contextPropagators)
                .setTracerProvider(tracerProvider)
                .build();
        return new PyroscopeTelemetry(sdkTelemetry, pyroscopeTelemetryConfig);
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

    private Resource defaultResource(String applicationName) {
        if (applicationName == null) {
            return Resource.getDefault();
        }
        return Resource.getDefault()
                .merge(Resource.create(Attributes.of(ResourceAttributes.SERVICE_NAME, applicationName)));
    }

}
