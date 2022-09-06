package scooter

import (
	"context"
	"rideshare/utility"
)

func OrderScooter(ctx context.Context, searchRadius int64) {
	utility.FindNearestVehicle(ctx, searchRadius, "scooter")
}
