package utility

import (
	"os"
	"strconv"
	"time"
)

func EnvIntOrDefault(name string, n int) int {
	s, ok := os.LookupEnv(name)
	if !ok {
		return n
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return n
	}

	return v
}

func EnvDurationOrDefault(name string, d time.Duration) time.Duration {
	s, ok := os.LookupEnv(name)
	if !ok {
		return d
	}

	v, err := time.ParseDuration(s)
	if err != nil {
		return d
	}

	return v
}
