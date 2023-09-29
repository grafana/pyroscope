package utility

import (
	"context"
	"os"
	"time"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
)

const durationConstant = time.Duration(200 * time.Millisecond)

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

		if vehicle == "car" {
			checkDriverAvailability(searchRadius)
		}
	})
}

type LeakyPool struct {
	stopAll chan chan struct{}
	group   *errgroup.Group
}

func (l *LeakyPool) GoLeak(fn func() error) {
	done := make(chan struct{})

	// Leak a hanging goroutine.
	l.group.Go(func() error {
		// Run the function.
		err := fn()

		// Report the function is over.
		done <- struct{}{}

		force_goroutine_leak := time.Now().Minute()%2 == 0
		if force_goroutine_leak {
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

func (l *LeakyPool) cleanUp() {
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

// NewLeakyPool creates a goroutine pool of size n which will occasionally leak
// a goroutine. Periodically it will clean up the leaked goroutines. Adjust n to
// leak more or less resources.
func NewLeakyPool(n int) *LeakyPool {
	pool := &LeakyPool{
		stopAll: make(chan chan struct{}, n),
		group:   &errgroup.Group{},
	}
	pool.group.SetLimit(n)

	go pool.cleanUp()
	return pool
}
