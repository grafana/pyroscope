package main

import (
	"log"

	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

func work(n int) {
	for i := 0; i < n; i++ {

	}
}

func fastFunction() {
	work(2000)
}

func slowFunction() {
	work(8000)
}

func main() {
	profiler.Start(profiler.Config{
		ApplicationName: "simple.golang.app",
		ServerAddress:   "http://pyroscope:8080", // this will run inside docker-compose, hence `pyroscope` for hostname
	})

	log.Println("test")
	for {
		fastFunction()
		slowFunction()
	}
}
