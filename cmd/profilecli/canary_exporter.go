// Provenance-includes-location: https://github.com/prometheus/blackbox_exporter/blob/9d3e8e8ab443772aefb4ba2c3010329fd6d9be84/prober/http.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Prometheus Authors.

// This has been mostly adapted to our use case from the blackbox exporter

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log/level"
	"github.com/google/go-cmp/cmp"
	gprofile "github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/atomic"

	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/pprof/testhelper"
)

type canaryExporterParams struct {
	*phlareClient
	ListenAddress string
	TestFrequency time.Duration
}

func addCanaryExporterParams(ceCmd commander) *canaryExporterParams {
	var (
		params = &canaryExporterParams{}
	)
	ceCmd.Flag("listen-address", "Listen address for the canary exporter.").Default(":4101").StringVar(&params.ListenAddress)
	ceCmd.Flag("test-frequency", "How often the specified Pyroscope cell should be tested.").Default("15s").DurationVar(&params.TestFrequency)
	params.phlareClient = addPhlareClient(ceCmd)

	return params
}

type canaryExporter struct {
	params *canaryExporterParams
	reg    *prometheus.Registry
	mux    *http.ServeMux

	defaultTransport http.RoundTripper
	metrics          *canaryExporterMetrics

	hostname string
}

type canaryExporterMetrics struct {
	success                                 *prometheus.GaugeVec
	duration                                *prometheus.HistogramVec
	contentLength                           *prometheus.GaugeVec
	bodyUncompressedLength                  *prometheus.GaugeVec
	statusCode                              *prometheus.GaugeVec
	isSSL                                   prometheus.Gauge
	probeSSLEarliestCertExpiry              prometheus.Gauge
	probeSSLLastChainExpiryTimestampSeconds prometheus.Gauge
	probeTLSVersion                         *prometheus.GaugeVec
	probeSSLLastInformation                 *prometheus.GaugeVec
	probeHTTPVersion                        *prometheus.GaugeVec
}

func newCanaryExporterMetrics(reg prometheus.Registerer) *canaryExporterMetrics {
	return &canaryExporterMetrics{
		success: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_success",
			Help: "Duration of http request by phase, summed over all redirects",
		}, []string{"name"}),
		duration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "probe_http_duration_seconds",
			Help:    "Duration of http request by phase, summed over all redirects",
			Buckets: prometheus.ExponentialBuckets(0.00025, 4, 10),
		}, []string{"name", "phase"}),

		contentLength: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_http_content_length",
			Help: "Length of http content response",
		}, []string{"name"}),
		bodyUncompressedLength: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_http_uncompressed_body_length",
			Help: "Length of uncompressed response body",
		}, []string{"name"}),
		statusCode: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_http_status_code",
			Help: "Response HTTP status code",
		}, []string{"name"}),
		isSSL: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "probe_http_ssl",
			Help: "Indicates if SSL was used for the final redirect",
		}),
		probeSSLEarliestCertExpiry: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "probe_ssl_earliest_cert_expiry",
			Help: "Returns last SSL chain expiry in unixtime",
		}),
		probeSSLLastChainExpiryTimestampSeconds: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "probe_ssl_last_chain_expiry_timestamp_seconds",
			Help: "Returns last SSL chain expiry in timestamp",
		}),
		probeTLSVersion: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "probe_tls_version_info",
				Help: "Returns the TLS version used or NaN when unknown",
			},
			[]string{"version"},
		),
		probeSSLLastInformation: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "probe_ssl_last_chain_info",
				Help: "Contains SSL leaf certificate information",
			},
			[]string{"fingerprint_sha256", "subject", "issuer", "subjectalternative"},
		),
		probeHTTPVersion: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_http_version",
			Help: "Returns the version of HTTP of the probe response",
		}, []string{"name"}),
	}
}

func newCanaryExporter(params *canaryExporterParams) *canaryExporter {
	// Disable keepalives messing with probes
	defaultTransport := http.DefaultTransport.(*http.Transport)
	defaultTransport.DisableKeepAlives = true
	params.defaultTransport = defaultTransport

	reg := prometheus.NewRegistry()
	ce := &canaryExporter{
		reg:    reg,
		mux:    http.NewServeMux(),
		params: params,

		hostname:         "unknown",
		defaultTransport: params.httpClient().Transport,

		metrics: newCanaryExporterMetrics(reg),
	}

	metricsPath := "/metrics"
	ce.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
			<head><title>Pyroscope Blackbox Exporter</title></head>
			<body>
			<h1>Pyroscope Blackbox Exporter</h1>
			<p><a href="` + metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	// Expose the registered metrics via HTTP.
	ce.mux.Handle(metricsPath, promhttp.HandlerFor(
		ce.reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
			// Pass custom registry
			Registry: ce.reg,
		},
	))

	if hostname, err := os.Hostname(); err == nil {
		ce.hostname = hostname
	}

	return ce
}

func (ce *canaryExporter) run(ctx context.Context) error {

	run := func(ctx context.Context) {
		if err := ce.testPyroscopeCell(ctx); err != nil {
			for _, line := range strings.Split(err.Error(), "\n") {
				level.Error(logger).Log("msg", "error testing pyroscope cell", "err", line)
			}
		}
	}
	run(ctx)

	go func() {
		ticker := time.NewTicker(ce.params.TestFrequency)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case n := <-ticker.C:
				cCtx, cancel := context.WithDeadline(ctx, n.Add(ce.params.TestFrequency))
				run(cCtx)
				cancel()
			}
		}
	}()

	if err := http.ListenAndServe(ce.params.ListenAddress, ce.mux); err != nil {
		return err
	}

	return nil
}

func (ce *canaryExporter) doTrace(ctx context.Context, probeName string) (rCtx context.Context, done func(bool)) {
	level.Info(logger).Log("msg", "starting probe", "probeName", probeName)
	tt := newInstrumentedTransport(ce.defaultTransport, ce.metrics, probeName)
	ce.params.client.Transport = tt

	trace := &httptrace.ClientTrace{
		DNSStart:             tt.DNSStart,
		DNSDone:              tt.DNSDone,
		ConnectStart:         tt.ConnectStart,
		ConnectDone:          tt.ConnectDone,
		GotConn:              tt.GotConn,
		GotFirstResponseByte: tt.GotFirstResponseByte,
		TLSHandshakeStart:    tt.TLSHandshakeStart,
		TLSHandshakeDone:     tt.TLSHandshakeDone,
	}
	rCtx = httptrace.WithClientTrace(ctx, trace)

	return rCtx, func(result bool) {
		// At this point body is fully read and we can write end time.
		tt.current.end = time.Now()

		// record body size
		ce.metrics.bodyUncompressedLength.WithLabelValues(probeName).Set(float64(tt.bodySize.Load()))

		// aggregate duration for all requests (that is to support redirects)
		durations := make(map[string]float64)

		for _, trace := range tt.traces {
			durations["resolve"] += trace.dnsDone.Sub(trace.start).Seconds()

			// Continue here if we never got a connection because a request failed.
			if trace.gotConn.IsZero() {
				continue
			}

			if trace.tls {
				// dnsDone must be set if gotConn was set.
				durations["connect"] += trace.connectDone.Sub(trace.dnsDone).Seconds()
				durations["tls"] += trace.tlsDone.Sub(trace.tlsStart).Seconds()
			} else {
				durations["connect"] += trace.gotConn.Sub(trace.dnsDone).Seconds()
			}

			// Continue here if we never got a response from the server.
			if trace.responseStart.IsZero() {
				continue
			}
			durations["processing"] += trace.responseStart.Sub(trace.gotConn).Seconds()

			// Continue here if we never read the full response from the server.
			// Usually this means that request either failed or was redirected.
			if trace.end.IsZero() {
				continue
			}
			durations["transfer"] += trace.end.Sub(trace.responseStart).Seconds()
		}

		// now store the values in the histogram
		for phase, value := range durations {
			ce.metrics.duration.WithLabelValues(probeName, phase).Observe(value)
		}

		if m := ce.metrics.success.WithLabelValues(probeName); result {
			level.Info(logger).Log("msg", "probe successful", "probeName", probeName)
			m.Set(1)
		} else {
			level.Info(logger).Log("msg", "probe failed", "probeName", probeName)
			m.Set(0)
		}
	}
}

func (ce *canaryExporter) testPyroscopeCell(ctx context.Context) error {

	now := time.Now()
	p := testhelper.NewProfileBuilder(now.UnixNano())
	p.Labels = p.Labels[:0]
	p.CustomProfile("deadmans_switch", "made_up", "profilos", "made_up", "profilos")
	p.WithLabels(
		"job", "canary-exporter",
		"instance", ce.hostname,
	)
	p.UUID = uuid.New()
	p.ForStacktraceString("func1", "func2").AddSamples(10)
	p.ForStacktraceString("func1").AddSamples(20)

	data, err := p.Profile.MarshalVT()
	if err != nil {
		return err
	}

	// ingest a fake profile
	err = func() error {
		rCtx, done := ce.doTrace(ctx, "ingest")
		result := false
		defer func() {
			done(result)
		}()

		if _, err := ce.params.pusherClient().Push(rCtx, connect.NewRequest(&pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{
				{
					Labels: p.Labels,
					Samples: []*pushv1.RawSample{{
						ID:         uuid.New().String(),
						RawProfile: data,
					}},
				},
			},
		})); err != nil {
			return err
		}

		result = true
		return err
	}()
	if err != nil {
		return fmt.Errorf("error during ingestion: %w", err)
	}

	level.Info(logger).Log("msg", "successfully ingested profile", "uuid", p.UUID.String())

	// now try to query it back
	err = func() error {
		rCtx, done := ce.doTrace(ctx, "query-instant")
		result := false
		defer func() {
			done(result)
		}()

		respQueryInstant, err := ce.params.queryClient().SelectMergeProfile(rCtx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
			Start:         now.UnixMilli(),
			End:           now.Add(5 * time.Second).UnixMilli(),
			LabelSelector: fmt.Sprintf(`{job="canary-exporter", instance="%s"}`, ce.hostname),
			ProfileTypeID: "deadmans_switch:made_up:profilos:made_up:profilos",
		}))
		if err != nil {
			return err
		}

		buf, err := respQueryInstant.Msg.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "failed to marshal protobuf")
		}

		gp, err := gprofile.Parse(bytes.NewReader(buf))
		if err != nil {
			return errors.Wrap(err, "failed to parse profile")
		}

		expected := map[string]int64{
			"func1>func2": 10,
			"func1":       20,
		}
		actual := make(map[string]int64)

		var sb strings.Builder
		for _, s := range gp.Sample {
			sb.Reset()
			for _, loc := range s.Location {
				if sb.Len() != 0 {
					_, err := sb.WriteRune('>')
					if err != nil {
						return err
					}
				}
				for _, line := range loc.Line {
					_, err := sb.WriteString(line.Function.Name)
					if err != nil {
						return err
					}
				}
			}
			actual[sb.String()] = actual[sb.String()] + s.Value[0]
		}

		if diff := cmp.Diff(expected, actual); diff != "" {
			return fmt.Errorf("query instantly mismatch (-expected, +actual):\n%s", diff)
		}

		result = true
		return nil
	}()
	if err != nil {
		return fmt.Errorf("error during instant query probe: %w", err)
	}

	return nil

}

// roundTripTrace holds timings for a single HTTP roundtrip.
type roundTripTrace struct {
	tls           bool
	start         time.Time
	dnsDone       time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	end           time.Time
	tlsStart      time.Time
	tlsDone       time.Time
}

// transport is a custom transport keeping traces for each HTTP roundtrip.
type transport struct {
	Transport http.RoundTripper
	name      string
	metrics   *canaryExporterMetrics

	mu       sync.Mutex
	traces   []*roundTripTrace
	current  *roundTripTrace
	bodySize *atomic.Int64
}

func newInstrumentedTransport(rt http.RoundTripper, m *canaryExporterMetrics, name string) *transport {
	return &transport{
		Transport: rt,
		traces:    []*roundTripTrace{},
		name:      name,
		metrics:   m,
		bodySize:  atomic.NewInt64(0),
	}
}

type readerWrapper struct {
	io.ReadCloser
	bodySize *atomic.Int64
}

func (rw *readerWrapper) Read(p []byte) (n int, err error) {
	n, err = rw.ReadCloser.Read(p)
	rw.bodySize.Add(int64(n))
	return n, err
}

// RoundTrip switches to a new trace, then runs embedded RoundTripper.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	level.Debug(logger).Log("msg", "making HTTP request", "url", req.URL.String(), "host", req.Host)

	trace := &roundTripTrace{}
	if req.URL.Scheme == "https" {
		trace.tls = true
	}
	t.current = trace
	t.traces = append(t.traces, trace)

	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	resp.Body = &readerWrapper{ReadCloser: resp.Body, bodySize: t.bodySize}

	if resp.TLS != nil {
		t.metrics.isSSL.Set(float64(1))
		t.metrics.probeSSLEarliestCertExpiry.Set(float64(getEarliestCertExpiry(resp.TLS).Unix()))
		t.metrics.probeTLSVersion.WithLabelValues(getTLSVersion(resp.TLS)).Set(1)
		t.metrics.probeSSLLastChainExpiryTimestampSeconds.Set(float64(getLastChainExpiry(resp.TLS).Unix()))
		t.metrics.probeSSLLastInformation.WithLabelValues(getFingerprint(resp.TLS), getSubject(resp.TLS), getIssuer(resp.TLS), getDNSNames(resp.TLS)).Set(1)
	}

	t.metrics.statusCode.WithLabelValues(t.name).Set(float64(resp.StatusCode))
	t.metrics.contentLength.WithLabelValues(t.name).Set(float64(resp.ContentLength))

	var httpVersionNumber float64
	httpVersionNumber, err = strconv.ParseFloat(strings.TrimPrefix(resp.Proto, "HTTP/"), 64)
	if err != nil {
		level.Error(logger).Log("msg", "Error parsing version number from HTTP version", "err", err)
	}
	t.metrics.probeHTTPVersion.WithLabelValues(t.name).Set(httpVersionNumber)

	return resp, err
}

func (t *transport) DNSStart(_ httptrace.DNSStartInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.start = time.Now()
}
func (t *transport) DNSDone(_ httptrace.DNSDoneInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.dnsDone = time.Now()
}
func (ts *transport) ConnectStart(_, _ string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t := ts.current
	// No DNS resolution because we connected to IP directly.
	if t.dnsDone.IsZero() {
		t.start = time.Now()
		t.dnsDone = t.start
	}
}
func (t *transport) ConnectDone(net, addr string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.connectDone = time.Now()
}
func (t *transport) GotConn(_ httptrace.GotConnInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.gotConn = time.Now()
}
func (t *transport) GotFirstResponseByte() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.responseStart = time.Now()
}
func (t *transport) TLSHandshakeStart() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.tlsStart = time.Now()
}
func (t *transport) TLSHandshakeDone(_ tls.ConnectionState, _ error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.tlsDone = time.Now()
}
func getEarliestCertExpiry(state *tls.ConnectionState) time.Time {
	earliest := time.Time{}
	for _, cert := range state.PeerCertificates {
		if (earliest.IsZero() || cert.NotAfter.Before(earliest)) && !cert.NotAfter.IsZero() {
			earliest = cert.NotAfter
		}
	}
	return earliest
}

func getFingerprint(state *tls.ConnectionState) string {
	cert := state.PeerCertificates[0]
	fingerprint := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(fingerprint[:])
}

func getSubject(state *tls.ConnectionState) string {
	cert := state.PeerCertificates[0]
	return cert.Subject.String()
}

func getIssuer(state *tls.ConnectionState) string {
	cert := state.PeerCertificates[0]
	return cert.Issuer.String()
}

func getDNSNames(state *tls.ConnectionState) string {
	cert := state.PeerCertificates[0]
	return strings.Join(cert.DNSNames, ",")
}

func getLastChainExpiry(state *tls.ConnectionState) time.Time {
	lastChainExpiry := time.Time{}
	for _, chain := range state.VerifiedChains {
		earliestCertExpiry := time.Time{}
		for _, cert := range chain {
			if (earliestCertExpiry.IsZero() || cert.NotAfter.Before(earliestCertExpiry)) && !cert.NotAfter.IsZero() {
				earliestCertExpiry = cert.NotAfter
			}
		}
		if lastChainExpiry.IsZero() || lastChainExpiry.Before(earliestCertExpiry) {
			lastChainExpiry = earliestCertExpiry
		}

	}
	return lastChainExpiry
}

func getTLSVersion(state *tls.ConnectionState) string {
	switch state.Version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "unknown"
	}
}
