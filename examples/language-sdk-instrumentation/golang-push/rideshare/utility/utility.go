package utility

import (
	"context"
	"os"
	"time"

	"rideshare/rideshare"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

func checkDriverAvailability(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	startTime := time.Now()

	pool.Run(func() {
		for time.Since(startTime) < time.Duration(n)*durationConstant {
			i++
		}
	})

	// Every other minute this will artificially create make requests in eu-north region slow
	// this is just for demonstration purposes to show how performance impacts show up in the
	// flamegraph
	force_mutex_lock := time.Now().Minute()%2 == 0
	if os.Getenv("REGION") == "eu-north" && force_mutex_lock {
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
			checkDriverAvailability(searchRadius)
		}
	})
}
