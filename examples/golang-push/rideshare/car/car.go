package car

import "github.com/tree/main/examples/golang-push/rideshare/utility"

func OrderCar(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "car")
}
