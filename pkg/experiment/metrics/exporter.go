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
	config Config
	client remote.WriteClient
	data   map[AggregatedFingerprint]*TimeSeries
}

type Config struct {
	url      string
	username string
	password config.Secret
}

func NewExporter(tenant string, recordings []*Recording) *Exporter {
	cfg := configFromTenant(tenant)
	exporter := &Exporter{
		config: cfg,
		data:   map[AggregatedFingerprint]*TimeSeries{},
	}
	for _, r := range recordings {
		for fp, ts := range r.data {
			exporter.data[fp] = ts
		}
	}
	return exporter
}

func (e *Exporter) Send() error {
	if len(e.data) == 0 {
		return nil
	}
	if e.client == nil {
		e.client = newClient(e.config)
	}

	p := &prompb.WriteRequest{Timeseries: make([]prompb.TimeSeries, 0, len(e.data))}
	for _, ts := range e.data {
		pts := prompb.TimeSeries{
			Labels: make([]prompb.Label, 0, len(ts.Labels)),
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
	return e.client.Store(context.Background(), snappy.Encode(nil, buf.Bytes()), 0)
}

func newClient(cfg Config) remote.WriteClient {
	wURL, err := url.Parse(cfg.url)
	if err != nil {
		panic(err)
	}

	c, err := remote.NewWriteClient("pyroscope-metrics-exporter", &remote.ClientConfig{
		URL:     &config.URL{URL: wURL},
		Timeout: model.Duration(time.Second * 10),
		HTTPClientConfig: config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: cfg.username,
				Password: cfg.password,
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
	return c
}

func configFromTenant(tenant string) Config {
	// TODO
	return Config{
		url:      "omitted",
		username: "omitted",
		password: "omitted",
	}
}
