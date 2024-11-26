package compactor

import (
	"context"
	"net/url"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type WriteRequest struct {
	TimeSeries []TimeSeries
}

type TimeSeries struct {
	Labels  labels.Labels
	Samples []Sample
}

type Sample struct {
	Value     float64
	Timestamp int64
}

type Exporter struct {
	client remote.WriteClient
}

func NewExporter() *Exporter {
	wURL, err := url.Parse("https://prometheus-us-central1.grafana.net/api/prom/push")
	if err != nil {
		panic(err)
	}

	c, err := remote.NewWriteClient("exporter", &remote.ClientConfig{
		URL:     &config.URL{URL: wURL},
		Timeout: model.Duration(time.Second * 10),
		HTTPClientConfig: config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: "1741027",
				Password: "omitted",
			},
		},
		SigV4Config:      nil,
		AzureADConfig:    nil,
		Headers:          nil,
		RetryOnRateLimit: false,
	})
	if err != nil {
		panic(err)
	}
	return &Exporter{client: c}
}

func (e *Exporter) Send(ctx context.Context, req *WriteRequest) error {
	p := &prompb.WriteRequest{Timeseries: make([]prompb.TimeSeries, 0, len(req.TimeSeries))}
	for _, ts := range req.TimeSeries {
		// TODO: Merge external labels.
		pts := prompb.TimeSeries{
			Labels: make([]prompb.Label, 0, len(ts.Labels)-1),
		}
		for _, l := range ts.Labels {
			pts.Labels = append(pts.Labels, prompb.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		for _, s := range ts.Samples {
			pts.Samples = append(pts.Samples, prompb.Sample{
				Value:     s.Value,
				Timestamp: s.Timestamp,
			})
		}
		p.Timeseries = append(p.Timeseries, pts)
	}
	buf := proto.NewBuffer(nil)
	if err := buf.Marshal(p); err != nil {
		return err
	}
	return e.client.Store(ctx, snappy.Encode(nil, buf.Bytes()), 0)
}
