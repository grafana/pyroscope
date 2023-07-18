package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
)

// generate a range to iterate over it
func makeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}

// list all primeNumber from 1 to n
func primeNumberFrom1To(to int) []int {
	if to == 0 {
		to = 100
	}

	a := [0]int{}
	primeNumbers := a[:]

	r1 := makeRange(1, to)
	for _, i := range r1 {
		if i > 1 {
			r2 := makeRange(2, i)
			for _, j := range r2 {
				if i != j {
					if divisible(i, j) {
						break
					}
				} else {
					primeNumbers = append(primeNumbers, i)
					break
				}
			}
		}
	}
	return primeNumbers
}

// test if i is divisible by j (integer division)
func divisible(i, j int) bool {
	return i%j == 0

}

func main() {
	// goroutine to start the server profile
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	// loop to continually search for prime numbers
	to := 10_000
	for {
		res := primeNumberFrom1To(to)
		fmt.Printf("there are %d prime numbers from 1 to %d\n", len(res), to)
		time.Sleep(500 * time.Millisecond)
	}
}
