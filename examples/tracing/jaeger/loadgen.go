package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"rideshare/rideshare"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var hosts = []string{
	"us-east-1",
	"us-west-1",
	"eu-west-1",
}

var vehicles = []string{
	"bike",
	"scooter",
	"car",
}

func main() {
	c := rideshare.ReadConfig()
	c.AppName = "load-generator"

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

	for {
		if err = orderVehicle(context.Background()); err != nil {
			fmt.Println(err)
		}
	}
}

func orderVehicle(ctx context.Context) error {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "OrderVehicle")
	defer span.End()

	// Spend some time on CPU.
	d := time.Duration(200+rand.Intn(200)) * time.Millisecond
	begin := time.Now()
	for {
		if time.Now().Sub(begin) > d {
			break
		}
	}

	host := hosts[rand.Intn(len(hosts))]
	vehicle := vehicles[rand.Intn(len(hosts))]
	span.SetAttributes(attribute.String("vehicle", vehicle))
	url := fmt.Sprintf("http://%s:5000/%s", host, vehicle)
	fmt.Println("requesting", url)

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
