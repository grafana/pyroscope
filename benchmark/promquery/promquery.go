package promquery

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func QueryRange(start, end time.Time) (error, string, string) {
	client, err := api.NewClient(api.Config{
		Address: "http://localhost:9091",
	})

	if err != nil {
		return err, "", ""
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r := v1.Range{
		Start: start,
		End:   end,
		//		Step:  time.Minute,
		Step: time.Second * 15,
	}

	result, warnings, err := v1api.QueryRange(ctx, `rate(pyroscope_http_request_duration_seconds_count{handler="/ingest"}[1m])`, r)
	if err != nil {
		return err, "", ""
	}

	return nil, fmt.Sprintf("Warnings: %v\n", warnings), fmt.Sprintf("Result:\n%v\n", result)
}
