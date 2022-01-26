package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
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

type config struct {
	appName                string
	pyroscopeServerAddress string
	honeycombDataset       string
	honeycombAPIKey        string
	useDebugTracer         bool
}

func bikeRoute(_ http.ResponseWriter, r *http.Request) {
	bike.OrderBike(r.Context(), 1)
}

func scooterRoute(_ http.ResponseWriter, r *http.Request) {
	scooter.OrderScooter(r.Context(), 2)
}

func carRoute(_ http.ResponseWriter, r *http.Request) {
	car.OrderCar(r.Context(), 3)
}

func index(w http.ResponseWriter, r *http.Request) {
	b := bytes.NewBufferString("<h1>environment vars:</h1>")
	for _, env := range os.Environ() {
		b.WriteString(env + "<br>")
	}
	_, _ = b.WriteTo(w)
}

func main() {
	c := readConfig()

	// Configure profiler.
	p, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: c.appName,
		ServerAddress:   c.pyroscopeServerAddress,
		Logger:          pyroscope.StandardLogger,
		// In this scenario, labels should be set at tracing level:
		// Pyroscope won't store the labels other than profile_id.
		// Tags: map[string]string{"region": os.Getenv("REGION")},
	})
	if err != nil {
		log.Fatalf("failed to initialize profiler: %v\n", err)
	}
	defer func() {
		_ = p.Stop()
	}()

	// Setup tracing.
	var tp *sdktrace.TracerProvider
	if os.Getenv("DEBUG_TRACER") == "1" {
		// The tracer does not send traces but prints them to stdout.
		tp = initTracerProviderDebug()
	} else {
		tp = initTracerProviderHoneycomb(c)
	}

	// Set the Tracer Provider and the W3C Trace Context propagator as globals.
	// We wrap the tracer provider to also annotate goroutines with Span ID so
	// that pprof would add corresponding labels to profiling samples.
	otel.SetTracerProvider(newTracerProfilerProvider(tp,
		withProfileURLBuilder(defaultProfileURLBuilder(c.pyroscopeServerAddress, c.appName)),
		withRootSpanOnly(),
	))

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "indexHandler"))
	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(bikeRoute), "bikeHandler"))
	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(scooterRoute), "scooterHandler"))
	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(carRoute), "carHandler"))

	if err := http.ListenAndServe(":5000", nil); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func readConfig() config {
	c := config{
		appName:                "ride-sharing-app",
		pyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		honeycombDataset:       os.Getenv("HONEYCOMB_DATASET"),
		honeycombAPIKey:        os.Getenv("HONEYCOMB_API_KEY"),
		useDebugTracer:         os.Getenv("DEBUG_TRACER") == "1",
	}

	if !c.useDebugTracer {
		if c.honeycombDataset == "" {
			c.pyroscopeServerAddress = "ExampleDataset"
		}
		if c.honeycombAPIKey == "" {
			log.Fatalln("Honeycomb API key should be provided via HONEYCOMB_API_KEY env variable.")
		}
	}

	if c.pyroscopeServerAddress == "" {
		c.pyroscopeServerAddress = "http://localhost:4040"
	} else {
		u, err := url.Parse(c.pyroscopeServerAddress)
		if err != nil {
			log.Fatalf("Pyroscope server address is invalid: %v, must be a valid URL\n", err)
		}
		u.RawQuery = ""
		c.pyroscopeServerAddress = u.String()
	}

	return c
}

func initTracerProviderDebug() *sdktrace.TracerProvider {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("failed to initialize stdouttrace exporter %v\n", err)
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp)))
}

func initTracerProviderHoneycomb(c config) *sdktrace.TracerProvider {
	ctx := context.Background()
	// Configure a new exporter using environment variables for sending data to Honeycomb over gRPC.
	exp, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team":    c.honeycombAPIKey,
			"x-honeycomb-dataset": c.honeycombDataset,
		})),
	)

	if err != nil {
		log.Fatalf("failed to initialize exporter: %v\n", err)
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	// Note that ServiceNameKey attribute can include chars not allowed in Pyroscope
	// application name, therefore it should be used carefully.
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(c.appName),
		)),
	)
}

// Should be part of the pyroscope client package.
const profileIDLabelName = "profile_id"

var (
	profileIDSpanAttributeKey  = attribute.Key("pyroscope.profile.id")
	profileURLSpanAttributeKey = attribute.Key("pyroscope.profile.url")
)

// tracerProfilerProvider wraps spans with pprof tags.
type tracerProfilerProvider struct {
	tp trace.TracerProvider

	rootOnly bool
	profileURLBuilder
}

type profileURLBuilder func(profileID string) string

type option func(*tracerProfilerProvider)

// withRootSpanOnly indicates that only the root span is to be profiled.
// The profile includes samples captured during child span execution
// but the spans won't have their own profiles and won't be annotated
// with pyroscope.profile attributes.
func withRootSpanOnly() option {
	return func(tp *tracerProfilerProvider) {
		tp.rootOnly = true
	}
}

// withProfileURLBuilder specifies how profile URL is to be built. Optional.
func withProfileURLBuilder(b profileURLBuilder) option {
	return func(tp *tracerProfilerProvider) {
		tp.profileURLBuilder = b
	}
}

func defaultProfileURLBuilder(addr string, app string) profileURLBuilder {
	return func(id string) string {
		return addr + "?query=" + app + ".cpu%7Bprofile_id%3D%22" + id + "%22%7D"
	}
}

func newTracerProfilerProvider(tp trace.TracerProvider, options ...option) trace.TracerProvider {
	p := tracerProfilerProvider{tp: tp}
	for _, o := range options {
		o(&p)
	}
	return &p
}

func (w tracerProfilerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &tracerProfiler{p: w, tr: w.tp.Tracer(name, opts...)}
}

type tracerProfiler struct {
	p  tracerProfilerProvider
	tr trace.Tracer
}

func (w tracerProfiler) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if w.p.rootOnly && !isRootSpan(trace.SpanContextFromContext(ctx)) {
		return w.tr.Start(ctx, spanName, opts...)
	}
	var s spanWrapper
	ctx, s.Span = w.tr.Start(ctx, spanName, opts...)
	span := trace.SpanFromContext(ctx)
	profileID := trace.SpanContextFromContext(ctx).SpanID().String()
	// By this profiles can be easily associated with the corresponding spans.
	// We use span ID as a profile ID because it perfectly fits profiling scope.
	// In practice, a profile ID is an arbitrary string identifying the execution
	// scope that is associated with a tracing span.
	span.SetAttributes(profileIDSpanAttributeKey.String(profileID))
	// Optionally specify the profile URL.
	if w.p.profileURLBuilder != nil {
		span.SetAttributes(profileURLSpanAttributeKey.String(w.p.profileURLBuilder(profileID)))
	}
	// Store the context before attaching pprof labels.
	// The context is to be restored on End call.
	s.ctx = ctx
	ctx = pprof.WithLabels(ctx, pprof.Labels(profileIDLabelName, profileID))
	pprof.SetGoroutineLabels(ctx)
	return ctx, &s
}

var emptySpanID trace.SpanID

func isRootSpan(s trace.SpanContext) bool { return s.SpanID() == emptySpanID }

type spanWrapper struct {
	trace.Span
	ctx context.Context
}

func (s spanWrapper) End(options ...trace.SpanEndOption) {
	s.Span.End(options...)
	pprof.SetGoroutineLabels(s.ctx)
}
