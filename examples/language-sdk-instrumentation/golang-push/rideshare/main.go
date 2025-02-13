package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/rideshare"
	"rideshare/scooter"
	"rideshare/utility"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	otellogs "github.com/agoda-com/opentelemetry-logs-go"
	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	mmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
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
	rideshare.Log.Print(r.Context(), "showing index")
	result := "<h1>environment vars:</h1>"
	for _, env := range os.Environ() {
		result += env + "<br>"
	}
	w.Write([]byte(result))
}

func main() {
	config := rideshare.ReadConfig()

	tp, _ := setupOTEL(config)
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	p, err := rideshare.Profiler(config)

	if err != nil {
		log.Fatalf("error starting pyroscope profiler: %v", err)
	}

	defer func() {
		_ = p.Stop()
	}()

	histogram, err := otel.GetMeterProvider().Meter("histogram").Float64Histogram(
		"handler.duration",
		mmetric.WithDescription("The duration of handler execution."),
		mmetric.WithUnit("s"),
	)
	if err != nil {
		panic(err)
	}

	cleanup := utility.InitWorkerPool(config)
	defer cleanup()

	rideshare.Log.Print(context.Background(), "started ride-sharing app")

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "IndexHandler"))

	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		bikeRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds())
	}), "BikeHandler"))

	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		scooterRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds())
	}), "ScooterHandler"))

	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		carRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds())
	}), "CarHandler"))

	addr := fmt.Sprintf(":%s", config.RideshareListenPort)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func setupOTEL(c rideshare.Config) (tp *sdktrace.TracerProvider, err error) {
	tp, err = rideshare.TracerProvider(c)
	if err != nil {
		return nil, err
	}

	lp, err := rideshare.LoggerProvider(c)
	if err != nil {
		return nil, err
	}
	otellogs.SetLoggerProvider(lp)

	const (
		instrumentationName    = "otel/zap"
		instrumentationVersion = "0.0.1"
	)

	// Set the Tracer Provider and the W3C Trace Context propagator as globals.
	// We wrap the tracer provider to also annotate goroutines with Span ID so
	// that pprof would add corresponding labels to profiling samples.
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp))

	// Register the trace context and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create resource.
	res, err := newResource()
	if err != nil {
		return nil, err
	}

	// Create a meter provider.
	// You can pass this instance directly to your instrumented code if it
	// accepts a MeterProvider instance.
	mp, err := newMeterProvider(res)
	if err != nil {
		return nil, err
	}

	otel.SetMeterProvider(mp)

	return tp, err
}

func newResource() (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("rideshare-service"),
			semconv.ServiceVersion("0.1.0"),
		))
}

func newMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			metric.WithInterval(3*time.Second))),
	)

	return meterProvider, nil
}
