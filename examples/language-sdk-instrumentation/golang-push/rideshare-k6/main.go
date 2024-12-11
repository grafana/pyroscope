package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/rideshare"
	"rideshare/scooter"
	"rideshare/utility"

	otellogs "github.com/agoda-com/opentelemetry-logs-go"
	otelpyroscope "github.com/grafana/otel-profiling-go"
	"github.com/grafana/pyroscope-go/x/k6"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

	routes := []route{
		{"/", "IndexHandler", http.HandlerFunc(index)},
		{"/bike", "BikeHandler", http.HandlerFunc(bikeRoute)},
		{"/scooter", "ScooterHandler", http.HandlerFunc(scooterRoute)},
		{"/car", "CarHandler", http.HandlerFunc(carRoute)},
	}

	routes = applyOtelMiddleware(routes)
	routes = applyK6Middleware(routes)
	registerRoutes(http.DefaultServeMux, routes)

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

	return tp, err
}

type route struct {
	Path    string
	Name    string
	Handler http.Handler
}

func registerRoutes(mux *http.ServeMux, handlers []route) {
	for _, handler := range handlers {
		mux.Handle(handler.Path, handler.Handler)
	}
}

func applyOtelMiddleware(routes []route) []route {
	for i, route := range routes {
		routes[i].Handler = otelhttp.NewHandler(route.Handler, route.Name)
	}
	return routes
}

// applyK6Middleware adds the k6 instrumentation middleware to all routes. This
// enables the Pyroscope SDK to label the profiles with k6 test metadata.
func applyK6Middleware(routes []route) []route {
	for i, route := range routes {
		routes[i].Handler = k6.LabelsFromBaggageHandler(route.Handler)
	}
	return routes
}
