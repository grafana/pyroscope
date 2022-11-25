package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

var (
	HOSTS = []string{
		"us-east",
		"eu-north",
		"ap-south",
	}

	VEHICLES = []string{
		"bike",
		"scooter",
		"car",
	}
)

func main() {
	fmt.Println("starting load generator")
	time.Sleep(3 * time.Second)

	// every second
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			go func() {
				host := HOSTS[rand.Intn(len(HOSTS))]
				vehicle := VEHICLES[rand.Intn(len(VEHICLES))]
				http.Get(fmt.Sprintf("http://%s:3000/%s", host, vehicle))
			}()
		}
	}
}
