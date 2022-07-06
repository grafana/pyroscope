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

	if vehicle == "bike" && time.Now().Minute()%10 == 0 {	
		checkBikeAvailability(ctx, searchRadius)
	}
}

func checkBikeAvailability(ctx context.Context, n int64) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "CheckBikeAvailability")
	defer span.End()

	burnCPU(n * 3)
}

func checkDriverAvailability(ctx context.Context, n int64) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "CheckDriverAvailability")
	defer span.End()

	region := os.Getenv("REGION")

	burnCPU(n / 2)
	// Every 4 minutes this will artificially make requests in eu-north region slow
	// this is just for demonstration purposes to show how performance impacts show
	// up in the flamegraph.
	if region == "eu-north" && time.Now().Minute()*4%8 == 0 {
		burnCPU(n * 2)
	}
}

func burnCPU(n int64) {
	var v int
	for i := int64(0); i < n*2; i++ {
		for j := 0; j < 1<<30; j++ {
			v++
		}
	}
}
