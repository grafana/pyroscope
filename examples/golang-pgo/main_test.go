package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var vehicles = []string{
	"bike",
	"scooter",
	"car",
}

const (
	delayBetweenRequests = 2 * time.Millisecond
)

var client *http.Client

func BenchmarkApp(b *testing.B) {
	host := "http://localhost:5001"
	client = &http.Client{Transport: http.DefaultTransport}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			vehicle := vehicles[i%len(vehicles)]
			time.Sleep(delayBetweenRequests)
			err := orderVehicle(context.Background(), host, vehicle)
			require.NoError(b, err)
		}
	}
}

func orderVehicle(ctx context.Context, baseURL, vehicle string) error {
	url := fmt.Sprintf("%s/%s", baseURL, vehicle)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
