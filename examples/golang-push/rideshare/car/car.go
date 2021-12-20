package car

import "github.com/pyroscope-io/pyroscope/tree/main/examples/golang/utility"

func OrderCar(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "car")
}
