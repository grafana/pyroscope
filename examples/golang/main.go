package main

import (
	"context"
	"os"
	"runtime/pprof"

	"github.com/pyroscope-io/client/pyroscope"
)

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	// revive:enable:empty-block
}

func fastFunction(c context.Context) {
	pyroscope.TagWrapper(c, pyroscope.Labels("function", "fast"), func(c context.Context) {
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
	serverAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if serverAddress == "" {
		serverAddress = "http://localhost:4040"
	}
	pyroscope.Start(pyroscope.Config{
		ApplicationName: "simple.golang.app",
		ServerAddress:   serverAddress,
		Logger:          pyroscope.StandardLogger,
	})
	pyroscope.TagWrapper(context.Background(), pyroscope.Labels("foo", "bar"), func(c context.Context) {
		for {
			fastFunction(c)
			slowFunction(c)
		}
	})
}
