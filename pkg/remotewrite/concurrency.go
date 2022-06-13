package remotewrite

import "runtime"

const queueFactor = 4
const maxWorkers = 64

// This number is used for the HTTP client as well as the client queue
func numWorkers() int {
	v := runtime.NumCPU() * queueFactor
	if v > maxWorkers {
		return maxWorkers
	}
	return v
}
