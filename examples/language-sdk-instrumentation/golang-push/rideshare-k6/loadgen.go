package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"rideshare/rideshare"
	"rideshare/utility"
)

var vehicles = []string{
	"bike",
	"scooter",
	"car",
}

var client *http.Client

func main() {
	c := rideshare.ReadConfig()
	c.AppName = "load-generator"

	url, ok := os.LookupEnv("RIDESHARE_URL")
	if !ok {
		log.Fatalf("RIDESHARE_URL is not set")
	}

	vus := utility.EnvIntOrDefault("VUS", 1)
	jitter := utility.EnvDurationOrDefault("JITTER", 100*time.Millisecond)
	sleep := utility.EnvDurationOrDefault("SLEEP", 100*time.Millisecond)

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

	// Set the Tracer Provider and the W3C Trace Context propagator as globals.
	// We wrap the tracer provider to also annotate goroutines with Span ID so
	// that pprof would add corresponding labels to profiling samples.
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp))

	// Register the trace context and baggage propagators so data is propagated
	// across services/processes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	client = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	doneChs := make([]chan struct{}, vus)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	for i := 0; i < vus; i++ {
		doneChs[i] = make(chan struct{})

		go func(done <-chan struct{}) {
			for {
				select {
				case <-done:
					return
				default:
					err := sendThrottledRequest(context.Background(), url, sleep, jitter)
					if err != nil {
						log.Printf("failed to send request to %s: %v", url, err)
					}
				}
			}
		}(doneChs[i])
	}

	<-sig
	for _, done := range doneChs {
		close(done)
	}
}

func sendThrottledRequest(ctx context.Context, baseURL string, sleep time.Duration, jitter time.Duration) error {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "SendRequest")
	defer span.End()

	vehicle := vehicles[rand.Intn(len(vehicles))]
	err := orderVehicle(ctx, baseURL, vehicle)
	if err != nil {
		return fmt.Errorf("failed to order %s: %v", vehicle, err)
	}

	jitter = time.Duration(rand.Intn(int(jitter)))
	time.Sleep(sleep + jitter)
	return nil
}

func orderVehicle(ctx context.Context, baseURL, vehicle string) error {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "OrderVehicle")
	defer span.End()

	span.SetAttributes(attribute.String("vehicle", vehicle))
	url := fmt.Sprintf("%s/%s", baseURL, vehicle)
	log.Printf("requesting %s", url)

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
