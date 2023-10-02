package utility

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
)

const (
	durationConstant = time.Duration(200 * time.Millisecond)
)

var (
	// vehicles contains all the known vehicle types.
	vehicles []Vehicle
)

func init() {
	// Populate the vehicles database.

	const (
		scooterCount = 100_000
		bikeCount    = 100_000
		carCount     = 50_000
	)

	vehicles = make([]Vehicle, 0, scooterCount+bikeCount+carCount)
	for i := 0; i < scooterCount; i++ {
		vehicles = append(vehicles, Vehicle{
			ID:   int(rand.Int()),
			Type: "scooter",
		})
	}
	for i := 0; i < bikeCount; i++ {
		vehicles = append(vehicles, Vehicle{
			ID:   int(rand.Int()),
			Type: "bike",
		})
	}
	for i := 0; i < carCount; i++ {
		vehicles = append(vehicles, Vehicle{
			ID:   int(rand.Int()),
			Type: "car",
		})
	}

	// Randomly order to the vehicles.
	rand.Shuffle(len(vehicles), func(i, j int) {
		vehicles[i], vehicles[j] = vehicles[j], vehicles[i]
	})
}

type Vehicle struct {
	ID   int
	Type string
}

func mutexLock(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	startTime := time.Now()

	// This changes the amplitude of cpu bars
	for time.Since(startTime) < time.Duration(n*30)*durationConstant {
		i++
	}
}

func checkDriverAvailability(n int64) {
	var i int64 = 0

	// start time is number of seconds since epoch
	startTime := time.Now()

	for time.Since(startTime) < time.Duration(n)*durationConstant {
		i++
	}

	// Every other minute this will artificially create make requests in eu-north region slow
	// this is just for demonstration purposes to show how performance impacts show up in the
	// flamegraph
	force_mutex_lock := time.Now().Minute()%2 == 0
	if os.Getenv("REGION") == "eu-north" && force_mutex_lock {
		mutexLock(n)
	}

}

func FindNearestVehicle(ctx context.Context, searchRadius int64, vehicle string) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "FindNearestVehicle")
	span.SetAttributes(attribute.String("vehicle", vehicle))
	defer span.End()

	pyroscope.TagWrapper(ctx, pyroscope.Labels("vehicle", vehicle), func(ctx context.Context) {
		var i int64 = 0

		startTime := time.Now()
		for time.Since(startTime) < time.Duration(searchRadius)*durationConstant {
			i++
		}

		// Simulate finding an ID.
		ids := vehicleIDsFor(ctx, vehicle)
		_ = ids[rand.Intn(len(ids))]

		if vehicle == "car" {
			checkDriverAvailability(searchRadius)
		}
	})
}

func vehicleIDsFor(ctx context.Context, vehicleType string) []int {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "vehicleIDsFor")
	defer span.End()

	ids := make([]int, 0)
	for _, vehicle := range vehicles {
		if vehicle.Type != vehicleType {
			continue
		}
		ids = append(ids, vehicle.ID)
	}
	return ids
}

type RequestPool struct {
	stopAll chan chan struct{}
	group   *errgroup.Group
}

func (l *RequestPool) Handle(fn func() error) {
	done := make(chan struct{})

	// Leak a hanging goroutine.
	l.group.Go(func() error {
		// Run the function.
		err := fn()

		// Report the function is over.
		done <- struct{}{}

		force_leak := true
		if force_leak {
			// Report we leaked so we can eventually clean up and not
			// continuously crash the pod.
			stop := make(chan struct{})
			l.stopAll <- stop

			// Hang this goroutine until we're told to clean up.
			<-stop
		}
		return err
	})

	// Wait for function to finish.
	<-done
	close(done)
}

func (l *RequestPool) cleanUp() {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		<-ticker.C
		if len(l.stopAll) < cap(l.stopAll) {
			// Don't clean up yet.
			continue
		}

		// Recover all hanging goroutines.
		for c := range l.stopAll {
			c <- struct{}{}
		}
	}
}

// NewRequestPool creates a goroutine pool of size n which can leak goroutines.
// Periodically it will clean up the leaked goroutines. Adjust n to leak more or
// less resources.
func NewRequestPool(n int) *RequestPool {
	pool := &RequestPool{
		stopAll: make(chan chan struct{}, n),
		group:   &errgroup.Group{},
	}
	pool.group.SetLimit(n)

	go pool.cleanUp()
	return pool
}
