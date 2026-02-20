package pyroscope

import (
	"context"
	"io"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	otelpyroscope "github.com/grafana/otel-profiling-go"
	wwtracing "github.com/grafana/dskit/tracing"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// initTracing initializes the OTel TracerProvider.
//
// It delegates to dskit's NewOTelOrJaegerFromEnv, but if that fails
// due to a resource schema URL conflict (OTel SDK version mismatch
// with dskit's bundled semconv), it falls back to a direct init.
func initTracing(serviceName string, logger log.Logger, profilingEnabled bool) (io.Closer, error) {
	var opts []wwtracing.OTelOption
	if !profilingEnabled {
		opts = append(opts, wwtracing.WithPyroscopeDisabled())
	}
	closer, err := wwtracing.NewOTelOrJaegerFromEnv(serviceName, logger, opts...)
	if err == nil {
		return closer, nil
	}
	level.Warn(logger).Log("msg", "dskit tracing init failed, falling back to direct init", "err", err)
	return initTracingDirect(serviceName, logger, profilingEnabled)
}

// initTracingDirect creates the OTel TracerProvider directly, bypassing
// dskit's NewResource which may fail due to schema URL conflicts between
// the OTel SDK's bundled semconv and dskit's imported semconv version.
func initTracingDirect(serviceName string, logger log.Logger, profilingEnabled bool) (io.Closer, error) {
	exp, err := autoexport.NewSpanExporter(context.Background())
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, err
	}

	tpsdk := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(res),
	)

	var tp trace.TracerProvider = tpsdk
	if profilingEnabled {
		tp = otelpyroscope.NewTracerProvider(tp)
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(wwtracing.OTelPropagatorsFromEnv()...))
	otel.SetErrorHandler(otelErrorHandler{logger: logger})

	return &tracerProviderCloser{tp: tpsdk}, nil
}

type otelErrorHandler struct{ logger log.Logger }

func (h otelErrorHandler) Handle(err error) {
	level.Error(h.logger).Log("msg", "OpenTelemetry.ErrorHandler", "err", err)
}

type tracerProviderCloser struct{ tp *tracesdk.TracerProvider }

func (c *tracerProviderCloser) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.tp.Shutdown(ctx)
}
