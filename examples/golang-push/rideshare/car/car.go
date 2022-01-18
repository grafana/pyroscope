package car

import "rideshare/utility"

func OrderCar(searchRadius int64) {
	utility.FindNearestVehicle(searchRadius, "car")
}
