package utility

import (
	"context"
	"os"
	"time"

	"github.com/pyroscope-io/client/pyroscope"
)

func mutexLock(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	start_time := time.Now().Unix()

	for (time.Now().Unix() - start_time) < n*10 {
		i++
	}
}

func checkDriverAvailability(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	start_time := time.Now().Unix()

	for (time.Now().Unix() - start_time) < n/2 {
		i++
	}

	// Every 4 minutes this will artificially create make requests in us-west-1 region slow
	// this is just for demonstration purposes to show how performance impacts show up in the
	// flamegraph
	force_mutex_lock := time.Now().Minute()*4%8 == 0
	if os.Getenv("REGION") == "us-west-1" && force_mutex_lock {
		mutexLock(n)
	}

}

func FindNearestVehicle(searchRadius int64, vehicle string) {
	pyroscope.TagWrapper(context.Background(), pyroscope.Labels("vehicle", vehicle), func(ctx context.Context) {
		var i int64 = 0

		start_time := time.Now().Unix()
		for (time.Now().Unix() - start_time) < searchRadius {
			i++
		}

		if vehicle == "car" {
			checkDriverAvailability(searchRadius)
		}
	})
}
