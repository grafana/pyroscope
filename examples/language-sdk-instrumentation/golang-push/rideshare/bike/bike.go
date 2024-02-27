package bike

import (
	"context"
	"rideshare/rideshare"
	"rideshare/utility"
)

func OrderBike(ctx context.Context, searchRadius int64) {
	rideshare.Log.Printf(ctx, "ordering bike, with searchRadius=%d", searchRadius)
	utility.FindNearestVehicle(ctx, searchRadius, "bike")
}
