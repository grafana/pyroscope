package car

import "rideshare/utility"

func OrderCar(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "car")
}
