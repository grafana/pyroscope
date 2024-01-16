package rideshare

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs/otlplogshttp"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/stdout/stdoutlogs"
	"github.com/agoda-com/opentelemetry-logs-go/logs"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

type loggerAdapter struct {
	logger logs.Logger
}

func (la *loggerAdapter) Print(ctx context.Context, msg string) {
	severityNumber := logs.INFO
	now := time.Now()
	c := logs.LogRecordConfig{
		ObservedTimestamp: now,
		Timestamp:         &now,
		SeverityNumber:    &severityNumber,
		Body:              &msg,
	}

	span := trace.SpanFromContext(ctx)
	if spanID := span.SpanContext().SpanID(); spanID.IsValid() {
		c.SpanId = &spanID
	}
	if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
		c.TraceId = &traceID
	}
	la.logger.Emit(logs.NewLogRecord(c))
}

func (la *loggerAdapter) Printf(ctx context.Context, format string, v ...interface{}) {
	la.Print(ctx, fmt.Sprintf(format, v...))
}

var Log = &loggerAdapter{logger: nil}

type Config struct {
	AppName                    string
	PyroscopeServerAddress     string
	PyroscopeAuthToken         string // for OG pyroscope and cloudstorage
	PyroscopeBasicAuthUser     string // for grafana
	PyroscopeBasicAuthPassword string // for grafana

	OTLPUrl               string
	OTLPInsecure          bool
	OTLPBasicAuthUser     string
	OTLPBasicAuthPassword string
	OTLPTracesUrlPath     string

	UseDebugTracer bool
	UseDebugLogger bool
	Tags           map[string]string

	ParametersPoolSize       int
	ParametersPoolBufferSize int
}

func ReadConfig() Config {
	appName := os.Getenv("PYROSCOPE_APPLICATION_NAME")
	if appName == "" {
		appName = "ride-sharing-app"
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	c := Config{
		AppName:                    appName,
		PyroscopeServerAddress:     os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
		PyroscopeBasicAuthUser:     os.Getenv("PYROSCOPE_BASIC_AUTH_USER"),
		PyroscopeBasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),

		OTLPUrl:               os.Getenv("OTLP_URL"),
		OTLPInsecure:          os.Getenv("OTLP_INSECURE") == "1",
		OTLPBasicAuthUser:     os.Getenv("OTLP_BASIC_AUTH_USER"),
		OTLPBasicAuthPassword: os.Getenv("OTLP_BASIC_AUTH_PASSWORD"),
		OTLPTracesUrlPath:     os.Getenv("OTLP_TRACES_URL_PATH"),

		UseDebugTracer: os.Getenv("DEBUG_TRACER") == "1",
		UseDebugLogger: os.Getenv("DEBUG_LOGGER") == "1",
		Tags: map[string]string{
			"region":   os.Getenv("REGION"),
			"hostname": hostname,
		},

		ParametersPoolSize: envIntOrDefault("PARAMETERS_POOL_SIZE", 1000),

		// Internally, we represent this as buffer size in bytes, but to make it
		// more readable from as an env var, we represent the env var value in
		// kb.
		ParametersPoolBufferSize: envIntOrDefault("PARAMETERS_POOL_BUFFER_SIZE_KB", 1000) * 1000,
	}
	if c.AppName == "" {
		c.AppName = "ride-sharing-app"
	}
	if c.PyroscopeServerAddress == "" {
		c.PyroscopeServerAddress = "http://localhost:4040"
	}
	return c
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func newResource(c Config, extraAttrs ...attribute.KeyValue) *resource.Resource {
	host, _ := os.Hostname()

	attrs := append([]attribute.KeyValue{
		semconv.ServiceNameKey.String(c.AppName),
		semconv.CloudRegionKey.String(os.Getenv("REGION")),
		semconv.HostName(host),
	}, extraAttrs...)
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		// Note that ServiceNameKey attribute can include chars not allowed in Pyroscope
		// application name, therefore it should be used carefully.
		attrs...,
	)
}

func LoggerProvider(c Config) (*sdklogs.LoggerProvider, error) {
	if c.UseDebugLogger || c.OTLPUrl == "" {
		consoleProvider, err := stdoutlogs.NewExporter(stdoutlogs.WithWriter(os.Stderr))
		if err != nil {
			return nil, err
		}
		loggerProvider := sdklogs.NewLoggerProvider(
			sdklogs.WithBatcher(consoleProvider),
			sdklogs.WithResource(newResource(c)),
		)
		Log.logger = loggerProvider.Logger(
			"ride-share",
			logs.WithInstrumentationVersion("0.0.1"),
			logs.WithSchemaURL(semconv.SchemaURL),
		)
		return loggerProvider, nil
	}

	exporter, _ := otlplogs.NewExporter(context.Background(), otlplogs.WithClient(otlplogshttp.NewClient(
		otlplogshttp.WithEndpoint(c.OTLPUrl),
		otlplogshttp.WithURLPath("/otlp/v1/logs"),
		otlplogshttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + basicAuth(c.OTLPBasicAuthUser, c.OTLPBasicAuthPassword),
		}),
	)))

	// tell loki to use host.name and cloud.region as labels
	lokiHint := attribute.KeyValue{
		Key: attribute.Key("loki.resource.labels"),
		Value: attribute.StringSliceValue([]string{
			string(semconv.CloudRegionKey.String("").Key),
			string(semconv.ServiceNameKey.String("").Key),
			string(semconv.HostName("").Key),
		}),
	}

	loggerProvider := sdklogs.NewLoggerProvider(
		sdklogs.WithBatcher(exporter),
		sdklogs.WithResource(newResource(c, lokiHint)),
	)

	Log.logger = loggerProvider.Logger(
		"ride-share",
		logs.WithInstrumentationVersion("0.0.1"),
		logs.WithSchemaURL(semconv.SchemaURL),
	)

	return loggerProvider, nil
}

func TracerProvider(c Config) (*sdktrace.TracerProvider, error) {
	if c.UseDebugTracer || c.OTLPUrl == "" {
		return debugTracerProvider()
	}
	ctx := context.Background()
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(c.OTLPUrl)}
	if c.OTLPTracesUrlPath != "" {
		opts = append(opts, otlptracehttp.WithURLPath(c.OTLPTracesUrlPath))
	}
	if c.OTLPInsecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if c.OTLPBasicAuthUser != "" {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + basicAuth(c.OTLPBasicAuthUser, c.OTLPBasicAuthPassword),
		}))
	}

	exp, err := otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, err
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	tp2 := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(newResource(c)),
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

// envIntOrDefault looks up the environment variable key and returns the value
// as an int.
func envIntOrDefault(key string, fallback int) int {
	s, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}

	return v
}
