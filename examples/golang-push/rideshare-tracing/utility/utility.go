package utility

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func mutexLock(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	start_time := time.Now().Unix()

	for (time.Now().Unix() - start_time) < n*10 {
		i++
	}
}

func checkDriverAvailability(ctx context.Context, n int64) {
	tracer := otel.GetTracerProvider().Tracer("")
	ctx, span := tracer.Start(ctx, "checkDriverAvailability")
	defer span.End()

	var i int64

	// start time is number of seconds since epoch
	startTime := time.Now().Unix()
	for (time.Now().Unix() - startTime) < n/2 {
		i++
	}

	// Every 4 minutes this will artificially create make requests in us-west-1 region slow
	// this is just for demonstration purposes to show how performance impacts show up in the
	// flamegraph
	force_mutex_lock := time.Now().Minute()*4%8 == 0
	if os.Getenv("REGION") == "us-west-1" && force_mutex_lock {
		mutexLock(n)
	}
}

func FindNearestVehicle(ctx context.Context, searchRadius int64, vehicle string) {
	tracer := otel.GetTracerProvider().Tracer("")
	ctx, span := tracer.Start(ctx, "FindNearestVehicle")
	span.SetAttributes(attribute.String("vehicle", vehicle))
	defer span.End()

	var i int64

	start_time := time.Now().Unix()
	for (time.Now().Unix() - start_time) < searchRadius {
		i++
	}

	if vehicle == "car" {
		checkDriverAvailability(ctx, searchRadius)
	}
}
