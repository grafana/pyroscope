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
	"github.com/grafana/dskit/instrument"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
)

type StaticExporter struct {
	client remote.WriteClient
	wg     sync.WaitGroup

	logger log.Logger

	metrics *clientMetrics
}

const (
	envVarRemoteUrl      = "PYROSCOPE_METRICS_EXPORTER_REMOTE_URL"
	envVarRemoteUser     = "PYROSCOPE_METRICS_EXPORTER_REMOTE_USER"
	envVarRemotePassword = "PYROSCOPE_METRICS_EXPORTER_REMOTE_PASSWORD"

	metricsExporterUserAgent = "pyroscope-metrics-exporter"
)

func NewStaticExporterFromEnvVars(logger log.Logger, reg prometheus.Registerer) (Exporter, error) {
	remoteUrl := os.Getenv(envVarRemoteUrl)
	user := os.Getenv(envVarRemoteUser)
	password := os.Getenv(envVarRemotePassword)
	if remoteUrl == "" || user == "" || password == "" {
		return nil, fmt.Errorf("unable to load metrics exporter configuration, %s, %s and %s must be defined",
			envVarRemoteUrl, envVarRemoteUser, envVarRemotePassword)
	}
	metrics := newMetrics(reg, remoteUrl)

	client, err := newClient(remoteUrl, user, password, metrics)
	if err != nil {
		return nil, err
	}

	return &StaticExporter{
		client:  client,
		wg:      sync.WaitGroup{},
		logger:  logger,
		metrics: metrics,
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

func newClient(remoteUrl, user, password string, m *clientMetrics) (remote.WriteClient, error) {
	wURL, err := url.Parse(remoteUrl)
	if err != nil {
		return nil, err
	}

	client, err := remote.NewWriteClient("exporter", &remote.ClientConfig{
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
	if err != nil {
		return nil, err
	}
	t := client.(*remote.Client).Client.Transport
	client.(*remote.Client).Client.Transport = promhttp.InstrumentRoundTripperDuration(m.requestDuration, t)
	return client, nil
}

type clientMetrics struct {
	requestDuration *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer, remoteUrl string) *clientMetrics {
	m := &clientMetrics{
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Subsystem: "metrics_exporter",
			Name:      "client_request_duration_seconds",
			Help:      "Time (in seconds) spent on remote_write",
			Buckets:   instrument.DefBuckets,
		}, []string{}),
	}
	if reg != nil {
		// TODO(alsoba13): include tenant label once we have a client per tenant/datasource in place
		remoteUrlReg := prometheus.WrapRegistererWith(prometheus.Labels{"url": remoteUrl}, reg)
		remoteUrlReg.MustRegister(
			m.requestDuration,
		)
	}
	return m
}
