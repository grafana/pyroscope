package scooter

import "rideshare/utility"

func OrderScooter(searchRadius int64) {
	utility.FindNearestVehicle(searchRadius, "scooter")
}
