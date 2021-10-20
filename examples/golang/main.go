package main

import (
	"context"
	"fmt"
	"runtime/pprof"

	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	fmt.Printf("work\n")
	// revive:enable:empty-block
}

func fastFunction() {
	profiler.WithLabels(profiler.Labels{"function": "fast"}, func() {
		work(20000000)
	})
}

func slowFunction() {
	// Standard pprof.Do wrapper works as well. A context with
	// the current labels can be retrieved via profiler.Context call:
	ctx := profiler.Context(context.Background())
	pprof.Do(ctx, pprof.Labels("function", "slow"), func(context.Context) {
		work(80000000)
	})
}

func main() {
	profiler.Start(profiler.Config{
		ApplicationName: "simple.golang.app",
		ServerAddress:   "http://localhost:4040", // this will run inside docker-compose, hence `pyroscope` for hostname
	})
	profiler.WithLabels(profiler.Labels{"foo": "bar"}, func() {
		for {
			fastFunction()
			slowFunction()
		}
	})
}
