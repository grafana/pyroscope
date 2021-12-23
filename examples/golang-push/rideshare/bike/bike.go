package bike

import "github.com/tree/main/examples/golang-push/rideshare/utility"

func OrderBike(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "bike")
}
