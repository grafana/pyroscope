package main

import (
	"log"

	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

func work(int) {

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
		ServerAddress:   "http://pyroscope:4040", // this will run inside docker-compose, hence `pyroscope` for hostname
	})

	log.Println("test")
	for {
		fastFunction()
		slowFunction()
	}
}
