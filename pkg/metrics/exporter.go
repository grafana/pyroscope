package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/grafana/dskit/instrument"
	"github.com/klauspost/compress/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"

	pyroscopemodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type StaticExporter struct {
	client remote.WriteClient
	wg     sync.WaitGroup

	logger log.Logger

	metrics *clientMetrics
}

const (
	metricsExporterUserAgent = "pyroscope-metrics-exporter"
)

func NewExporter(remoteWriteAddress string, logger log.Logger, reg prometheus.Registerer) (Exporter, error) {
	metrics := newMetrics(reg, remoteWriteAddress)
	client, err := newClient(remoteWriteAddress, metrics)
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

func (e *StaticExporter) Send(tenantId string, data []prompb.TimeSeries) error {
	e.wg.Add(1)
	go func(data []prompb.TimeSeries) {
		defer e.wg.Done()
		p := &prompb.WriteRequest{Timeseries: data}
		buf := proto.NewBuffer(nil)
		if err := buf.Marshal(p); err != nil {
			level.Error(e.logger).Log("msg", "unable to marshal prompb.WriteRequest", "err", err)
			return
		}

		ctx := tenant.InjectTenantID(context.Background(), tenantId)
		_, err := e.client.Store(ctx, snappy.Encode(nil, buf.Bytes()), 0)
		if err != nil {
			level.Error(e.logger).Log("msg", "unable to store prompb.WriteRequest", "err", err)
			return
		}
		seriesByRuleID := make(map[string]int)
		for _, ts := range data {
			ruleID := "unknown"
			for _, l := range ts.Labels {
				if l.Name == pyroscopemodel.RuleIDLabel {
					ruleID = l.Value
					break
				}
			}
			seriesByRuleID[ruleID]++
		}
		for ruleID, count := range seriesByRuleID {
			e.metrics.seriesSent.WithLabelValues(tenantId, ruleID).Add(float64(count))
		}
	}(data)
	return nil
}

func (e *StaticExporter) Flush() {
	e.wg.Wait()
}

func newClient(remoteUrl string, m *clientMetrics) (remote.WriteClient, error) {
	wURL, err := url.Parse(remoteUrl)
	if err != nil {
		return nil, err
	}

	client, err := remote.NewWriteClient("exporter", &remote.ClientConfig{
		URL:     &config.URL{URL: wURL},
		Timeout: model.Duration(time.Second * 10),
		Headers: map[string]string{
			"User-Agent": metricsExporterUserAgent,
		},
		RetryOnRateLimit: false,
	})
	if err != nil {
		return nil, err
	}
	t := client.(*remote.Client).Client.Transport
	client.(*remote.Client).Client.Transport = &RoundTripper{m, t}
	return client, nil
}

type clientMetrics struct {
	requestDuration *prometheus.HistogramVec
	requestBodySize *prometheus.CounterVec
	seriesSent      *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer, remoteUrl string) *clientMetrics {
	m := &clientMetrics{
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Subsystem: "metrics_exporter",
			Name:      "client_request_duration_seconds",
			Help:      "Time (in seconds) spent on remote_write",
			Buckets:   instrument.DefBuckets,
		}, []string{"route", "status_code", "tenant"}),
		requestBodySize: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "metrics_exporter",
			Name:      "request_message_bytes_total",
			Help:      "Size (in bytes) of messages sent on remote_write.",
		}, []string{"route", "status_code", "tenant"}),
		seriesSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "metrics_exporter",
			Name:      "series_sent_total",
			Help:      "Number of series sent on remote_write.",
		}, []string{"tenant", "rule_id"}),
	}
	if reg != nil {
		remoteUrlReg := prometheus.WrapRegistererWith(prometheus.Labels{"url": remoteUrl}, reg)
		remoteUrlReg.MustRegister(
			m.requestDuration,
			m.requestBodySize,
			m.seriesSent,
		)
	}
	return m
}

type RoundTripper struct {
	metrics *clientMetrics
	next    http.RoundTripper
}

func (m *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tenantId, err := tenant.ExtractTenantIDFromContext(req.Context())
	if err != nil {
		return nil, fmt.Errorf("unable to get tenant ID from context: %w", err)
	}
	req.Header.Set("X-Scope-OrgId", tenantId)

	start := time.Now()
	resp, err := m.next.RoundTrip(req)
	duration := time.Since(start)

	statusCode := ""
	if resp != nil {
		statusCode = strconv.Itoa(resp.StatusCode)
	}

	m.metrics.requestDuration.WithLabelValues(req.RequestURI, statusCode, tenantId).Observe(duration.Seconds())
	m.metrics.requestBodySize.WithLabelValues(req.RequestURI, statusCode, tenantId).Add(float64(req.ContentLength))
	return resp, err
}
