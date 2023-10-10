package car

import (
	"context"
	"rideshare/rideshare"
	"rideshare/utility"
)

func OrderCar(ctx context.Context, searchRadius int64) {
	rideshare.Log.Printf(ctx, "ordering car, with searchRadius=%d", searchRadius)
	utility.FindNearestVehicle(ctx, searchRadius, "car")
}
