package car

import (
	"context"

	"rideshare/ride"
)

func OrderCar(ctx context.Context, searchRadius int64) {
	ride.FindNearestVehicle(ctx, searchRadius, "car")
}
