package car

import (
	"context"
	"rideshare/utility"
)

func OrderCar(ctx context.Context, searchRadius int64) {
	utility.FindNearestVehicle(ctx, searchRadius, "car")
}
