package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
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
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	otelpyroscope "github.com/grafana/otel-profiling-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	mmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

	tp, lp, mp, _ := setupOTEL(config)
	defer func() {
		_ = tp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
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

	// Register Prometheus metrics handler
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":5001", nil); err != nil {
			slog.Error("metrics server error", "error", err)
		}
	}()

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "IndexHandler"))

	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		bikeRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds(),
			mmetric.WithAttributes(
				attribute.String("vehicle", "bike"),
				attribute.String("route", "/bike"),
			),
		)
	}), "BikeHandler"))

	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		scooterRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds(),
			mmetric.WithAttributes(
				attribute.String("vehicle", "scooter"),
				attribute.String("route", "/scooter"),
			),
		)
	}), "ScooterHandler"))

	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		carRoute(w, r)
		duration := time.Since(start)
		histogram.Record(r.Context(), duration.Seconds(),
			mmetric.WithAttributes(
				attribute.String("vehicle", "car"),
				attribute.String("route", "/car"),
			),
		)
	}), "CarHandler"))

	addr := fmt.Sprintf(":%s", config.RideshareListenPort)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func setupOTEL(c rideshare.Config) (tp *sdktrace.TracerProvider, lp *sdklogs.LoggerProvider, mp *sdkmetric.MeterProvider, err error) {
	tp, err = rideshare.TracerProvider(c)
	if err != nil {
		return nil, nil, nil, err
	}

	lp, err = rideshare.LoggerProvider(c)
	if err != nil {
		return nil, nil, nil, err
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

	mp, err = rideshare.MeterProvider(c)
	if err != nil {
		return nil, nil, nil, err
	}
	otel.SetMeterProvider(mp)

	return tp, lp, mp, err
}
