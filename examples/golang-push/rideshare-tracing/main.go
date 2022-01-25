package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"runtime/pprof"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/scooter"

	"github.com/pyroscope-io/client/pyroscope"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
)

func bikeRoute(w http.ResponseWriter, r *http.Request) {
	bike.OrderBike(r.Context(), 1)
	w.Write([]byte("<h1>Bike ordered</h1>"))
}

func scooterRoute(w http.ResponseWriter, r *http.Request) {
	scooter.OrderScooter(r.Context(), 2)
	w.Write([]byte("<h1>Scooter ordered</h1>"))
}

func carRoute(w http.ResponseWriter, r *http.Request) {
	car.OrderCar(r.Context(), 3)
	w.Write([]byte("<h1>Car ordered</h1>"))
}

func index(w http.ResponseWriter, r *http.Request) {
	result := "<h1>environment vars:</h1>"
	for _, env := range os.Environ() {
		result += env + "<br>"
	}
	w.Write([]byte(result))
}

func main() {
	appName := "ride-sharing-app"
	serverAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if serverAddress == "" {
		serverAddress = "http://localhost:4040"
	}
	p, _ := pyroscope.Start(pyroscope.Config{
		ApplicationName: appName,
		ServerAddress:   serverAddress,
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"region": os.Getenv("REGION")},
	})

	var tp *sdktrace.TracerProvider
	if os.Getenv("DEBUG_TRACER") == "1" {
		// The tracer does not send traces but prints them to stdout.
		tp = initTracerProviderDebug(appName)
	} else {
		tp = initTracerProviderHoneycomb(appName,
			os.Getenv("HONEYCOMB_API_KEY"),
			os.Getenv("HONEYCOMB_DATASET"))
	}

	defer func() {
		_ = tp.Shutdown(context.Background())
		_ = p.Stop()
	}()

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "indexHandler"))
	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(bikeRoute), "bikeHandler"))
	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(scooterRoute), "scooterHandler"))
	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(carRoute), "carHandler"))

	if err := http.ListenAndServe(":5000", nil); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func initTracerProviderDebug(appName string) *sdktrace.TracerProvider {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Panicf("failed to initialize stdouttrace exporter %v\n", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)))
	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(newTracerProfilerProvider(appName, tp))
	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	return tp
}

func initTracerProviderHoneycomb(appName, apiKey, dataset string) *sdktrace.TracerProvider {
	ctx := context.Background()
	// Configure a new exporter using environment variables for sending data to Honeycomb over gRPC.
	exp, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team":    apiKey,
			"x-honeycomb-dataset": dataset,
		})))

	if err != nil {
		log.Fatalf("failed to initialize exporter: %v", err)
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	// Note that ServiceNameKey attribute can include chars not allowed in Pyroscope
	// application name, therefore it should be used carefully.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(appName),
		)),
	)

	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(newTracerProfilerProvider(appName, tp))
	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp
}

// Should be part of the pyroscope client package.
const profileIDLabelName = "profile_id"

var profileIDSpanAttributeKey = attribute.Key("pyroscope.profile.id")

// tracerProfilerProvider wraps spans with pprof tags.
type tracerProfilerProvider struct {
	appName string
	tp      trace.TracerProvider
}

func newTracerProfilerProvider(appName string, tp trace.TracerProvider) trace.TracerProvider {
	return &tracerProfilerProvider{appName: appName, tp: tp}
}

func (w tracerProfilerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &tracerProfiler{appName: w.appName, tr: w.tp.Tracer(name, opts...)}
}

type tracerProfiler struct {
	appName string
	tr      trace.Tracer
}

func (w tracerProfiler) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	var s spanWrapper
	ctx, s.Span = w.tr.Start(ctx, spanName, opts...)
	span := trace.SpanFromContext(ctx)
	profileID := trace.SpanContextFromContext(ctx).SpanID().String()
	// By this profiles can be easily associated with the corresponding spans.
	// We use span ID as a profile ID because it perfectly fits profiling scope.
	// In practice, a profile ID is an arbitrary string identifying the execution
	// scope that is associated with a tracing span.
	span.SetAttributes(profileIDSpanAttributeKey.String(profileID))
	// Store the context before attaching pprof labels.
	// The context is to be restored on End call.
	s.ctx = ctx
	ctx = pprof.WithLabels(ctx, pprof.Labels(profileIDLabelName, profileID))
	pprof.SetGoroutineLabels(ctx)
	return ctx, &s
}

type spanWrapper struct {
	trace.Span
	ctx context.Context
}

func (s spanWrapper) End(options ...trace.SpanEndOption) {
	s.Span.End(options...)
	pprof.SetGoroutineLabels(s.ctx)
}
