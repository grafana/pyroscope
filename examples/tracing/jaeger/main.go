package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/rideshare"
	"rideshare/scooter"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func bikeRoute(_ http.ResponseWriter, r *http.Request) {
	bike.OrderBike(r.Context(), 1)
}

func scooterRoute(_ http.ResponseWriter, r *http.Request) {
	scooter.OrderScooter(r.Context(), 2)
}

func carRoute(_ http.ResponseWriter, r *http.Request) {
	car.OrderCar(r.Context(), 3)
}

func index(w http.ResponseWriter, r *http.Request) {
	b := bytes.NewBufferString("<h1>environment vars:</h1>")
	for _, env := range os.Environ() {
		b.WriteString(env + "<br>")
	}
	_, _ = b.WriteTo(w)
}

func main() {
	c := rideshare.ReadConfig()
	c.AppName = "ride-sharing-app"

	// Configure profiler.
	p, err := rideshare.Profiler(c)
	if err != nil {
		log.Fatalf("failed to initialize profiler: %v\n", err)
	}
	defer func() {
		_ = p.Stop()
	}()

	// Configure tracing.
	tp, err := rideshare.TracerProvider(c)
	if err != nil {
		log.Fatalf("failed to initialize profiler: %v\n", err)
	}
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(index), "IndexHandler"))
	http.Handle("/bike", otelhttp.NewHandler(http.HandlerFunc(bikeRoute), "BikeHandler"))
	http.Handle("/scooter", otelhttp.NewHandler(http.HandlerFunc(scooterRoute), "ScooterHandler"))
	http.Handle("/car", otelhttp.NewHandler(http.HandlerFunc(carRoute), "CarHandler"))

	if err = http.ListenAndServe(":5000", nil); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
