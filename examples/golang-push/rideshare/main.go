package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/scooter"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	otelpyroscope "github.com/pyroscope-io/otel-profiling-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
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
	result := "<h1>environment vars:</h1>"
	for _, env := range os.Environ() {
		result += env + "<br>"
	}
	w.Write([]byte(result))
}

func main() {
	serverAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if serverAddress == "" {
		serverAddress = "http://localhost:4040"
	}
	appName := os.Getenv("PYROSCOPE_APPLICATION_NAME")
	if appName == "" {
		appName = "ride-sharing-app"
	}

	tp, _ := setupTracing()
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: appName,
		ServerAddress:   serverAddress,
		AuthToken:       os.Getenv("PYROSCOPE_AUTH_TOKEN"),
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"region": os.Getenv("REGION")},
	})
	if err != nil {
		log.Fatalf("error starting pyroscope profiler: %v", err)
	}

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "IndexHandler"))
	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(bikeRoute), "BikeHandler"))
	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(scooterRoute), "ScooterHandler"))
	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(carRoute), "CarHandler"))

	log.Fatal(http.ListenAndServe(":5000", nil))
}

func setupTracing() (tp *sdktrace.TracerProvider, err error) {
	tp, err = tracerProviderDebug()
	if err != nil {
		return nil, err
	}

	// Set the Tracer Provider and the W3C Trace Context propagator as globals.
	// We wrap the tracer provider to also annotate goroutines with Span ID so
	// that pprof would add corresponding labels to profiling samples.
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp,
		otelpyroscope.WithAppName("ride-sharing-app"),
		otelpyroscope.WithRootSpanOnly(true),
		otelpyroscope.WithAddSpanName(true),
		otelpyroscope.WithPyroscopeURL("http://localhost:4040"),
		otelpyroscope.WithProfileBaselineLabels(map[string]string{"region": os.Getenv("REGION")}),
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

func tracerProviderDebug() (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exp))), nil
}
