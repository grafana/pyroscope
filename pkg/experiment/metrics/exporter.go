package metrics

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type Exporter interface {
	Send(tenant string, samples []prompb.TimeSeries) error
	Flush()
}

type StaticExporter struct {
	client remote.WriteClient
	wg     sync.WaitGroup

	logger log.Logger
}

const (
	envVarRemoteUrl      = "METRICS_EXPORTER_REMOTE_URL"
	envVarRemoteUser     = "METRICS_EXPORTER_REMOTE_USER"
	envVarRemotePassword = "METRICS_EXPORTER_REMOTE_PASSWORD"

	metricsExporterUserAgent = "pyroscope-metrics-exporter"
)

func NewStaticExporterFromEnvVars(logger log.Logger) (Exporter, error) {
	remoteUrl := os.Getenv(envVarRemoteUrl)
	user := os.Getenv(envVarRemoteUser)
	password := os.Getenv(envVarRemotePassword)
	if remoteUrl == "" || user == "" || password == "" {
		return nil, fmt.Errorf("unable to load metrics exporter configuration, %s, %s and %s must be defined",
			envVarRemoteUrl, envVarRemoteUser, envVarRemotePassword)
	}

	client, err := newClient(remoteUrl, user, password)
	if err != nil {
		return nil, err
	}

	return &StaticExporter{
		client: client,
		wg:     sync.WaitGroup{},
		logger: logger,
	}, nil
}

func (e *StaticExporter) Send(tenant string, data []prompb.TimeSeries) error {
	e.wg.Add(1)
	go func(data []prompb.TimeSeries) {
		defer e.wg.Done()
		p := &prompb.WriteRequest{Timeseries: data}
		buf := proto.NewBuffer(nil)
		if err := buf.Marshal(p); err != nil {
			level.Error(e.logger).Log("msg", "unable to marshal prompb.WriteRequest", "err", err)
			return
		}
		err := e.client.Store(context.Background(), snappy.Encode(nil, buf.Bytes()), 0)
		if err != nil {
			level.Error(e.logger).Log("msg", "unable to store prompb.WriteRequest", "err", err)
			return
		}
	}(data)
	return nil
}

func (e *StaticExporter) Flush() {
	e.wg.Wait()
}

func newClient(remoteUrl, user, password string) (remote.WriteClient, error) {
	wURL, err := url.Parse(remoteUrl)
	if err != nil {
		return nil, err
	}

	// TODO: enhance client with metrics
	return remote.NewWriteClient("exporter", &remote.ClientConfig{
		URL:     &config.URL{URL: wURL},
		Timeout: model.Duration(time.Second * 10),
		HTTPClientConfig: config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: user,
				Password: config.Secret(password),
			},
		},
		Headers: map[string]string{
			"User-Agent": metricsExporterUserAgent,
		},
		RetryOnRateLimit: false,
	})
}
