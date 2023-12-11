package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	_ "net/http/pprof" // Standard way of adding pprof to your server
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
	var wg sync.WaitGroup

	// Server for pprof
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	wg.Add(1) // pprof - so we won't exit prematurely

	wg.Add(4) // Adding 4 detective tasks
	go gatherClues(&wg)
	go analyzeEvidence(&wg)
	go interviewWitnesses(&wg)
	go chaseSuspect(&wg)
	go solveMystery(&wg)

	wg.Wait() // Wait for all detective tasks to complete
	fmt.Println("Mystery solved!")
}
