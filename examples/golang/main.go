package main

import (
	"context"
	"fmt"
	"log"
	"runtime/pprof"
	"time"

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

func fastFunction(c context.Context) {
	profiler.TagWrapper(c, profiler.Labels("function", "fast"), func(c context.Context) {
		work(20000000)
	})
}

func slowFunction(c context.Context) {
	// standard pprof.Do wrappers work as well
	pprof.Do(c, pprof.Labels("function", "slow"), func(c context.Context) {
		work(80000000)
	})
}

func main() {
	profiler.Start(profiler.Config{
		ApplicationName: "simple.golang.app",
		ServerAddress:   "http://localhost:4040", // this will run inside docker-compose, hence `pyroscope` for hostname
	})

	// recursion(0)
	profiler.TagWrapper(context.Background(), profiler.Labels("foo", "bar"), func(c context.Context) {
		for {
			fastFunction(c)
			slowFunction(c)
		}
	})
}

func recursion(n int) {
	if n%100000 == 0 {
		log.Printf("recursion %d", n)
		time.Sleep(time.Duration(n) * time.Microsecond)
	}
	recursion(n + 1)
}
