package rideshare

import (
	"context"
	"log"
	"net/url"
	"os"
	"runtime/pprof"

	"github.com/pyroscope-io/client/pyroscope"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
)

type Config struct {
	AppName                string
	PyroscopeServerAddress string
	PyroscopeProfileURL    string
	HoneycombDataset       string
	HoneycombAPIKey        string
	UseDebugTracer         bool
}

func ReadConfig() Config {
	c := Config{
		PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		PyroscopeProfileURL:    os.Getenv("PYROSCOPE_PROFILE_URL"),
		HoneycombDataset:       os.Getenv("HONEYCOMB_DATASET"),
		HoneycombAPIKey:        os.Getenv("HONEYCOMB_API_KEY"),
		UseDebugTracer:         os.Getenv("DEBUG_TRACER") == "1",
	}

	if !c.UseDebugTracer {
		if c.HoneycombDataset == "" {
			c.HoneycombDataset = "ExampleDataset"
		}
		if c.HoneycombAPIKey == "" {
			log.Fatalln("Honeycomb API key should be provided via HONEYCOMB_API_KEY env variable.")
		}
	}

	if c.PyroscopeServerAddress == "" {
		c.PyroscopeServerAddress = "http://localhost:4040"
	} else {
		u, err := url.Parse(c.PyroscopeServerAddress)
		if err != nil {
			log.Fatalf("Pyroscope server address is invalid: %v, must be a valid URL\n", err)
		}
		u.RawQuery = ""
		c.PyroscopeServerAddress = u.String()
	}

	if c.PyroscopeProfileURL == "" {
		c.PyroscopeProfileURL = c.PyroscopeServerAddress
	} else {
		u, err := url.Parse(c.PyroscopeProfileURL)
		if err != nil {
			log.Fatalf("Pyroscope server URL is invalid: %v\n", err)
		}
		u.RawQuery = ""
		c.PyroscopeProfileURL = u.String()
	}

	return c
}

func TracerProvider(c Config) (tp *sdktrace.TracerProvider, err error) {
	if c.UseDebugTracer {
		// The tracer does not send traces but prints them to stdout.
		tp, err = initTracerProviderDebug()
	} else {
		tp, err = initTracerProviderHoneycomb(c)
	}
	if err != nil {
		return nil, err
	}

	// Set the Tracer Provider and the W3C Trace Context propagator as globals.
	// We wrap the tracer provider to also annotate goroutines with Span ID so
	// that pprof would add corresponding labels to profiling samples.
	otel.SetTracerProvider(newTracerProfilerProvider(tp,
		withProfileURLBuilder(defaultProfileURLBuilder(c.PyroscopeProfileURL, c.AppName)),
		withRootSpanOnly(),
	))

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, err
}

func Profiler(c Config) (*pyroscope.Profiler, error) {
	return pyroscope.Start(pyroscope.Config{
		ApplicationName: c.AppName,
		ServerAddress:   c.PyroscopeServerAddress,
		Logger:          pyroscope.StandardLogger,
		// In this scenario, labels should be set at tracing level:
		// Pyroscope won't store the labels other than profile_id.
		// Tags: map[string]string{"region": os.Getenv("REGION")},
	})
}

func initTracerProviderDebug() (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp))), nil
}

func initTracerProviderHoneycomb(c Config) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()
	// Configure a new exporter using environment variables for sending data to Honeycomb over gRPC.
	exp, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team":    c.HoneycombAPIKey,
			"x-honeycomb-dataset": c.HoneycombDataset,
		})),
	)
	if err != nil {
		return nil, err
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	// Note that ServiceNameKey attribute can include chars not allowed in Pyroscope
	// application name, therefore it should be used carefully.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(c.AppName),
		)),
	)

	return tp, nil
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

	ctx, span := w.tr.Start(ctx, spanName, opts...)
	s := spanWrapper{
		profileID: trace.SpanContextFromContext(ctx).SpanID().String(),
		Span:      span,
		ctx:       ctx,
		p:         w.p,
	}

	ctx = pprof.WithLabels(ctx, pprof.Labels(profileIDLabelName, s.profileID))
	pprof.SetGoroutineLabels(ctx)
	return ctx, &s
}

var emptySpanID trace.SpanID

func isRootSpan(s trace.SpanContext) bool {
	return s.IsRemote() || s.SpanID() == emptySpanID
}

type spanWrapper struct {
	trace.Span
	ctx context.Context

	profileID string
	p         tracerProfilerProvider
}

func (s spanWrapper) End(options ...trace.SpanEndOption) {
	// By this profiles can be easily associated with the corresponding spans.
	// We use span ID as a profile ID because it perfectly fits profiling scope.
	// In practice, a profile ID is an arbitrary string identifying the execution
	// scope that is associated with a tracing span.
	s.SetAttributes(profileIDSpanAttributeKey.String(s.profileID))
	// Optionally specify the profile URL.
	if s.p.profileURLBuilder != nil {
		s.SetAttributes(profileURLSpanAttributeKey.String(s.p.profileURLBuilder(s.profileID)))
	}
	s.Span.End(options...)
	pprof.SetGoroutineLabels(s.ctx)
}
