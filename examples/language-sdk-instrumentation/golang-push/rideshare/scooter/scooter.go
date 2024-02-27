package scooter

import (
	"context"
	"rideshare/rideshare"
	"rideshare/utility"
)

func OrderScooter(ctx context.Context, searchRadius int64) {
	rideshare.Log.Printf(ctx, "ordering scooter, with searchRadius=%d", searchRadius)
	utility.FindNearestVehicle(ctx, searchRadius, "scooter")
}
