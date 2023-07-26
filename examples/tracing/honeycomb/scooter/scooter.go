package scooter

import (
	"context"

	"rideshare/ride"
)

func OrderScooter(ctx context.Context, searchRadius int64) {
	ride.FindNearestVehicle(ctx, searchRadius, "scooter")
}
