package scooter

import "rideshare/utility"

func OrderScooter(search_radius int64) {
	utility.FindNearestVehicle(search_radius, "scooter")
}
