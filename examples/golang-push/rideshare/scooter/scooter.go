package scooter

import "github.com/tree/main/examples/golang-push/rideshare/utility"

func OrderScooter(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "scooter")
}
