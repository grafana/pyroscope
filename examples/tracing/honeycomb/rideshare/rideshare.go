package rideshare

import (
	"context"
	"log"
	"net/url"
	"os"

	"github.com/pyroscope-io/client/pyroscope"
	"github.com/pyroscope-io/otelpyroscope"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"google.golang.org/grpc/credentials"
)

type Config struct {
	AppName                string
	PyroscopeServerAddress string
	PyroscopeProfileURL    string
	HoneycombDataset       string
	HoneycombAPIKey        string
	UseDebugTracer         bool
	Tags                   map[string]string
}

func ReadConfig() Config {
	c := Config{
		PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		PyroscopeProfileURL:    os.Getenv("PYROSCOPE_PROFILE_URL"),
		HoneycombDataset:       os.Getenv("HONEYCOMB_DATASET"),
		HoneycombAPIKey:        os.Getenv("HONEYCOMB_API_KEY"),
		UseDebugTracer:         os.Getenv("DEBUG_TRACER") == "1",
		Tags: map[string]string{
			"region": os.Getenv("REGION"),
		},
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
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp,
		otelpyroscope.WithAppName(c.AppName),
		otelpyroscope.WithRootSpanOnly(true),
		otelpyroscope.WithAddSpanName(true),
		otelpyroscope.WithPyroscopeURL(c.PyroscopeProfileURL),
		otelpyroscope.WithProfileBaselineLabels(c.Tags),
		otelpyroscope.WithProfileBaselineURL(true),
		otelpyroscope.WithProfileURL(true),
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
		Tags:            c.Tags,
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
