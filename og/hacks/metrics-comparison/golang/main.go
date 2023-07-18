package main

import (
	"log"

	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
	"os"
)

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	// revive:enable:empty-block
}

func fastFunction() {
	work(2000)
}

func slowFunction() {
	work(8000)
}

func main() {
	// allow overwrite pyroscope server url
	pyroscopeURL := os.Getenv("PYROSCOPE_URL")
	if pyroscopeURL == "" {
		pyroscopeURL = "pyroscope:4040" // this will run inside docker-compose, hence `pyroscope` for hostname
	}

	profiler.Start(profiler.Config{
		ApplicationName: "simple.golang.app",
		ServerAddress:   pyroscopeURL,
	})

	log.Println("test")
	for {
		fastFunction()
		slowFunction()
	}
}
