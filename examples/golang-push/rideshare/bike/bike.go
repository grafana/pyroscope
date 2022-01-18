package bike

import "rideshare/utility"

func OrderBike(searchRadius int64) {
	utility.FindNearestVehicle(searchRadius, "bike")
}
