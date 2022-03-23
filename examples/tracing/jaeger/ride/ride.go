package ride

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"rideshare/log"
)

func FindNearestVehicle(ctx context.Context, searchRadius int64, vehicle string) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "FindNearestVehicle")
	span.SetAttributes(attribute.String("vehicle", vehicle))
	defer span.End()

	logger := log.Logger(ctx).WithFields(logrus.Fields{
		"radius":  searchRadius,
		"vehicle": vehicle,
	})

	logger.Info("looking for nearest vehicle")
	burnCPU(searchRadius)
	if vehicle == "car" {
		checkDriverAvailability(ctx, searchRadius)
	}
}

func checkDriverAvailability(ctx context.Context, n int64) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "CheckDriverAvailability")
	defer span.End()

	region := os.Getenv("REGION")
	logger := log.Logger(ctx).WithField("region", region)
	logger.Info("checking for driver availability")

	burnCPU(n / 2)
	// Every 4 minutes this will artificially make requests in us-west-1 region slow
	// this is just for demonstration purposes to show how performance impacts show
	// up in the flamegraph.
	if region == "us-west-1" && time.Now().Minute()*4%8 == 0 {
		burnCPU(n * 2)
	}

	logger.Info("vehicle found")
}

func burnCPU(n int64) {
	var v int
	for i := int64(0); i < n*2; i++ {
		for j := 0; j < 1<<30; j++ {
			v++
		}
	}
}
