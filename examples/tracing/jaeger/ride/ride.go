package ride

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func FindNearestVehicle(ctx context.Context, searchRadius int64, vehicle string) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "FindNearestVehicle")
	span.SetAttributes(attribute.String("vehicle", vehicle))
	defer span.End()
	burnCPU(searchRadius)
	if vehicle == "car" {
		checkDriverAvailability(ctx, searchRadius)
	}
}

func checkDriverAvailability(ctx context.Context, n int64) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "CheckDriverAvailability")
	defer span.End()
	burnCPU(n / 2)
	// Every 4 minutes this will artificially make requests in us-west-1 region slow
	// this is just for demonstration purposes to show how performance impacts show
	// up in the flamegraph.
	if os.Getenv("REGION") == "us-west-1" && time.Now().Minute()*4%8 == 0 {
		burnCPU(n * 10)
	}
}

func burnCPU(n int64) {
	var i int64 = 0
	st := time.Now().Unix()
	for (time.Now().Unix() - st) < n {
		i++
	}
}
