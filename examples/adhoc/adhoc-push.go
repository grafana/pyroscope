package main

import (
	"math"
	"math/rand"

	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

func isPrime(n int64) bool {
	for i := int64(2); i <= n; i++ {
		if i*i > n {
			return true
		}
		if n%i == 0 {
			return false
		}
	}
	return false
}

func slow(n int64) int64 {
	sum := int64(0)
	for i := int64(1); i <= n; i++ {
		sum += i
	}
	return sum
}

func fast(n int64) int64 {
	sum := int64(0)
	root := int64(math.Sqrt(float64(n)))
	for a := int64(1); a <= n; a += root {
		b := a + root - 1
		if n < b {
			b = n
		}
		sum += (b - a + 1) * (a + b) / 2
	}
	return sum
}

func run() {
	base := rand.Int63n(1000000) + 1
	for i := int64(0); i < 40000000; i++ {
		n := rand.Int63n(10000) + 1
		if isPrime(base + i) {
			slow(n)
		} else {
			fast(n)
		}
	}
}

func main() {
	// No need to modify existing settings,
	// pyroscope will override the server address
	profiler.Start(profiler.Config{
		ApplicationName: "adhoc.example.go",
		ServerAddress:   "http://pyroscope:4040",
	})
	run()
}
