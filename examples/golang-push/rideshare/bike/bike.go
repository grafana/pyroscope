package bike

import "rideshare/utility"

func OrderBike(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "bike")
}
