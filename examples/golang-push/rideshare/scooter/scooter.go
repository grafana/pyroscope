package scooter

import "github.com/pyroscope-io/pyroscope/tree/main/examples/golang/utility"

func OrderScooter(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "scooter")
}
