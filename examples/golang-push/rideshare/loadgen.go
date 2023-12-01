package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"rideshare/rideshare"
)

var hosts = []string{}

var vehicles = []string{
	"bike",
	"scooter",
	"car",
}

var client *http.Client

func main() {
	c := rideshare.ReadConfig()
	c.AppName = "load-generator"
	hosts = os.Args[1:]
	if len(hosts) == 0 {
		hosts = []string{
			"us-east",
			"eu-north",
			"ap-south",
		}
	}

	groupByFactor := 3
	if os.Getenv("LOADGEN_GROUP_BY_FACTOR") != "" {
		var err error
		groupByFactor, err = strconv.Atoi(os.Getenv("LOADGEN_GROUP_BY_FACTOR"))
		if err != nil {
			log.Fatalf("issue with LOADGEN_GROUP_BY_FACTOR: %v\n", err)
		}
	}

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

	// Register the trace c ontext and baggage propagators so data is propagated across services/processes.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	client = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	groups := groupHosts(hosts, groupByFactor)
	for _, group := range groups {
		go func(group []string) {
			for {
				host := group[rand.Intn(len(group))]
				vehicle := vehicles[rand.Intn(len(vehicles))]
				if err = orderVehicle(context.Background(), host, vehicle); err != nil {
					fmt.Println(err)
				}
			}
		}(group)
	}

	select {}
}

func groupHosts(hosts []string, groupsOf int) [][]string {
	var res [][]string
	for i := 0; i < len(hosts); i += groupsOf {
		upperBoundary := i + groupsOf
		if upperBoundary > len(hosts) {
			upperBoundary = len(hosts)
		}
		res = append(res, hosts[i:upperBoundary])
	}
	return res
}

func orderVehicle(ctx context.Context, host, vehicle string) error {
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

	span.SetAttributes(attribute.String("vehicle", vehicle))
	url := fmt.Sprintf("http://%s:5000/%s", host, vehicle)
	fmt.Println("requesting", url)

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
