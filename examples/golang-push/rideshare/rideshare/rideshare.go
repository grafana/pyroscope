package rideshare

import (
	"context"
	"encoding/base64"
	"log"
	"net/url"
	"os"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	AppName                    string
	PyroscopeServerAddress     string
	PyroscopeAuthToken         string // for OG pyroscope and cloudstorage
	PyroscopeBasicAuthUser     string // for grafana
	PyroscopeBasicAuthPassword string // for grafana
	PyroscopeProfileURL        string
	OTLPUrl                    string
	OTLPBasicAuthUser          string
	OTLPBasicAuthPassword      string

	UseDebugTracer bool
	Tags           map[string]string
}

func ReadConfig() Config {
	c := Config{
		AppName:                    os.Getenv("PYROSCOPE_APPLICATION_NAME"),
		PyroscopeServerAddress:     os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		PyroscopeAuthToken:         os.Getenv("PYROSCOPE_AUTH_TOKEN"),
		PyroscopeBasicAuthUser:     os.Getenv("PYROSCOPE_BASIC_AUTH_USER"),
		PyroscopeBasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
		PyroscopeProfileURL:        os.Getenv("PYROSCOPE_PROFILE_URL"),
		OTLPUrl:                    os.Getenv("OTLP_URL"),
		OTLPBasicAuthUser:          os.Getenv("OTLP_BASIC_AUTH_USER"),
		OTLPBasicAuthPassword:      os.Getenv("OTLP_BASIC_AUTH_PASSWORD"),

		UseDebugTracer: os.Getenv("DEBUG_TRACER") == "1",
		Tags: map[string]string{
			"region": os.Getenv("REGION"),
		},
	}
	if c.AppName == "" {
		c.AppName = "ride-sharing-app"
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
			log.Fatalf("Pyroscope profile URL is invalid: %v\n", err)
		}
		u.RawQuery = ""
		c.PyroscopeProfileURL = u.String()
	}

	return c
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func TracerProvider(c Config) (*sdktrace.TracerProvider, error) {
	if c.OTLPUrl == "" {
		return debugTracerProvider()
	}
	ctx := context.Background()
	exp, err := otlptrace.New(ctx, otlptracehttp.NewClient(otlptracehttp.WithEndpoint(c.OTLPUrl),
		otlptracehttp.WithURLPath("/otlp/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + basicAuth(c.OTLPBasicAuthUser, c.OTLPBasicAuthPassword),
		})),
	)
	if err != nil {
		return nil, err
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	// Note that ServiceNameKey attribute can include chars not allowed in Pyroscope
	// application name, therefore it should be used carefully.
	tp2 := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(c.AppName),
			semconv.CloudRegionKey.String(os.Getenv("REGION")),
		)),
	)

	return tp2, nil
}

func debugTracerProvider() (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp))), nil
}

func Profiler(c Config) (*pyroscope.Profiler, error) {
	config := pyroscope.Config{
		ApplicationName: c.AppName,
		ServerAddress:   c.PyroscopeServerAddress,
		Logger:          pyroscope.StandardLogger,
		Tags:            c.Tags,
	}
	if c.PyroscopeAuthToken != "" {
		config.AuthToken = c.PyroscopeAuthToken
	} else if c.PyroscopeBasicAuthUser != "" {
		config.BasicAuthUser = c.PyroscopeBasicAuthUser
		config.BasicAuthPassword = c.PyroscopeBasicAuthPassword
	}
	return pyroscope.Start(config)
}
