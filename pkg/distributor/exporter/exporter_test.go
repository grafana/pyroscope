package exporter

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

func Test_Send(t *testing.T) {
	e := New()
	t.Log(e.Send(context.Background(), &WriteRequest{
		TimeSeries: []TimeSeries{{
			Labels: labels.Labels{
				{"__name__", "my_metric_name"},
				{"label_name", "label_value"},
			},
			Value: 1.0,
		}},
		ExternalLabels: nil,
		Timestamp:      time.Now().UnixMilli(),
	}))
}
