package utility

import (
	"context"
	"os"
	"time"

	"rideshare/rideshare"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
)

const durationConstant = time.Duration(200 * time.Millisecond)

var pool *workerPool

// InitWorkPool initializes the worker pool and returns a clean up function.
func InitWorkerPool(c rideshare.Config) func() {
	pool = newPool(c)
	return pool.Close
}

func mutexLock(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	startTime := time.Now()

	// This changes the amplitude of cpu bars
	for time.Since(startTime) < time.Duration(n*30)*durationConstant {
		i++
	}
}

func checkDriverAvailability(ctx context.Context, n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	startTime := time.Now()

	pool.Run(func() {
		for time.Since(startTime) < time.Duration(n)*durationConstant {
			i++
		}
	})

	// Get scenario from baggage and log it
	b := baggage.FromContext(ctx)
	scenario := b.Member("k6.scenario").Value()
	println("BAGGAGE_DEBUG: Scenario value from baggage:", scenario)

	// Check if we should force mutex lock based on region OR high load scenario
	force_mutex_lock := time.Now().Minute()%2 == 0 || scenario == "high_load"
	println("BAGGAGE_DEBUG: Force mutex lock:", force_mutex_lock, "Region:", os.Getenv("REGION"), "Scenario:", scenario)

	if os.Getenv("REGION") == "eu-north" && force_mutex_lock {
		println("BAGGAGE_DEBUG: Triggering mutex lock in eu-north")
		mutexLock(n)
	}
}

func FindNearestVehicle(ctx context.Context, searchRadius int64, vehicle string) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "FindNearestVehicle")
	span.SetAttributes(attribute.String("vehicle", vehicle))
	defer span.End()

	pyroscope.TagWrapper(ctx, pyroscope.Labels("vehicle", vehicle), func(ctx context.Context) {
		var i int64 = 0

		startTime := time.Now()
		for time.Since(startTime) < time.Duration(searchRadius)*durationConstant {
			i++
		}

		if vehicle == "car" {
			checkDriverAvailability(ctx, searchRadius)
		}
	})
}
