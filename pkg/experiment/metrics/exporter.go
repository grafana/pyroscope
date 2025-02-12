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

type Exporter struct{}

type Config struct {
	url      string
	username string
	password config.Secret
}

func (e *Exporter) Send(tenant string, data []prompb.TimeSeries) error {
	p := &prompb.WriteRequest{Timeseries: data}
	buf := proto.NewBuffer(nil)
	if err := buf.Marshal(p); err != nil {
		return err
	}
	cfg := configFromTenant(tenant)
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.Store(context.Background(), snappy.Encode(nil, buf.Bytes()), 0)
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

func configFromTenant(string) Config {
	// TODO
	return Config{
		url:      "omitted",
		username: "omitted",
		password: "omitted",
	}
}
