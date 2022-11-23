package bike

import (
	"context"
	"rideshare/utility"
)

func OrderBike(ctx context.Context, searchRadius int64) {
	utility.FindNearestVehicle(ctx, searchRadius, "bike")
}
