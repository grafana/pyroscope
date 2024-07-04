package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var dst = flag.String("dst", "http://localhost:8000", "Destination URL")
var nthreads = flag.Int("nworkers", 16, "Number of workers")

func main() {

	flag.Parse()

	sigChan := make(chan os.Signal, 3)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())

	fmt.Printf("Destination URL: %s\n", *dst)
	fmt.Printf("Starting %d workers\n", *nthreads)
	var wg sync.WaitGroup
	for i := 0; i < *nthreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(ctx)
		}()
	}
	go func() {
		fmt.Printf("Receiving SIGTERM\n")
		<-sigChan
		fmt.Println("Received SIGTERM, stopping workers")
		cancel()
	}()

	wg.Wait()
}

func run(ctx context.Context) {
	//fmt.Printf("Running worker\n")
	client := &http.Client{}
	success := 0
	errors := 0

	defer func() {
		fmt.Printf("Success: %d, Error: %d\n", success, errors)
	}()

	for {

		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := client.Get(*dst)
		if err != nil {
			errors++
			continue
		}
		if resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close() // Close the body to prevent resource leak
		}

		success++
	}

}
