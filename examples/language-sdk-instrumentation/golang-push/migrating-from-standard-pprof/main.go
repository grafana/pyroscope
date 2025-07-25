package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/grafana/pyroscope-go" // replace "net/http/pprof" with Pyroscope SDK
)

func busyWork(d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		// Busy loop
	}
}

func gatherClues(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Gathering clues...")
	busyWork(500 * time.Millisecond)
}

func analyzeEvidence(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Analyzing evidence...")
	busyWork(1 * time.Second)
}

func interviewWitnesses(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Interviewing witnesses...")
	busyWork(1 * time.Second)
}

func chaseSuspect(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Chasing the suspect...")
	busyWork(2 * time.Second)
}

func solveMystery(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Solving the mystery...")
	busyWork(2 * time.Second)
}

func main() {
	// These 2 lines are only required if you're using mutex or block profiling
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	// Pyroscope configuration
	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "detective.mystery.app",
		ServerAddress:   "https://profiles-prod-001.grafana.net", // If OSS, then "http://pyroscope.local:4040"
		// Optional HTTP Basic authentication
		// BasicAuthUser:     "<User>",     // 900009
		// BasicAuthPassword: "<Password>", // glc_SAMPLEAPIKEY0000000000==
		Logger: pyroscope.StandardLogger,
		Tags:   map[string]string{"hostname": os.Getenv("HOSTNAME")},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		log.Fatalf("Error starting profiler: %v", err)
	}
	defer func() {
		err := profiler.Stop()
		if err != nil {
			log.Printf("Error stopping profiler: %v", err)
		}
	}()

	// pyroscope.Start is non-blocking: the profiler will start shortly.
	// To ensure we don't miss the investigation, we wait briefly.
	time.Sleep(time.Second)

	var wg sync.WaitGroup
	wg.Add(5) // Adding 5 detective tasks
	go gatherClues(&wg)
	go analyzeEvidence(&wg)
	go interviewWitnesses(&wg)
	go chaseSuspect(&wg)
	go solveMystery(&wg)

	wg.Wait() // Wait for all detective tasks to complete
	fmt.Println("Mystery solved!")
}
