package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/rideshare"
	"rideshare/scooter"
	"rideshare/utility"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	otellogs "github.com/agoda-com/opentelemetry-logs-go"
	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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

	cleanup := utility.InitWorkerPool(config)
	defer cleanup()

	rideshare.Log.Print(context.Background(), "started ride-sharing app")

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "IndexHandler"))
	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(bikeRoute), "BikeHandler"))
	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(scooterRoute), "ScooterHandler"))
	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(carRoute), "CarHandler"))

	log.Fatal(http.ListenAndServe(":5000", nil))
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

	return tp, err
}
