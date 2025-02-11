package metrics

import (
	"context"
	"net/url"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type Exporter struct {
	client remote.WriteClient
	data   map[AggregatedFingerprint]*Sample
}

type Config struct {
	url      string
	username string
	password config.Secret
}

func NewExporter(tenant string, recordings []*Recording) (*Exporter, error) {
	exporter := &Exporter{
		data: map[AggregatedFingerprint]*Sample{},
	}
	for _, r := range recordings {
		for fp, ts := range r.data {
			exporter.data[fp] = ts
		}
	}
	if len(exporter.data) != 0 {
		cfg := configFromTenant(tenant)
		client, err := newClient(cfg)
		if err != nil {
			return nil, err
		}
		exporter.client = client
	}
	return exporter, nil
}

func (e *Exporter) Send() error {
	if e.client == nil {
		// no client = no data to send
		return nil
	}

	p := &prompb.WriteRequest{Timeseries: make([]prompb.TimeSeries, 0, len(e.data))}
	for _, sample := range e.data {
		pts := prompb.TimeSeries{
			Labels: make([]prompb.Label, 0, len(sample.Labels)),
			Samples: []prompb.Sample{
				{
					Value:     sample.Value,
					Timestamp: sample.Timestamp,
				},
			},
		}
		for _, l := range sample.Labels {
			pts.Labels = append(pts.Labels, prompb.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		p.Timeseries = append(p.Timeseries, pts)
	}
	buf := proto.NewBuffer(nil)
	if err := buf.Marshal(p); err != nil {
		return err
	}
	return e.client.Store(context.Background(), snappy.Encode(nil, buf.Bytes()), 0)
}

func newClient(cfg Config) (remote.WriteClient, error) {
	wURL, err := url.Parse(cfg.url)
	if err != nil {
		return nil, err
	}

	c, err := remote.NewWriteClient("exporter", &remote.ClientConfig{
		URL:     &config.URL{URL: wURL},
		Timeout: model.Duration(time.Second * 10),
		HTTPClientConfig: config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: cfg.username,
				Password: cfg.password,
			},
		},
		SigV4Config:   nil,
		AzureADConfig: nil,
		Headers: map[string]string{
			"User-Agent": "pyroscope-metrics-exporter",
		},
		RetryOnRateLimit: false,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func configFromTenant(tenant string) Config {
	// TODO
	return Config{
		url:      "omitted",
		username: "omitted",
		password: "omitted",
	}
}
