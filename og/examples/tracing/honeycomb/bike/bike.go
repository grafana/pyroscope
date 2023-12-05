package bike

import (
	"context"

	"rideshare/ride"
)

func OrderBike(ctx context.Context, searchRadius int64) {
	ride.FindNearestVehicle(ctx, searchRadius, "bike")
}
