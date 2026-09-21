package distributor

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	pyroscopeingest "github.com/grafana/pyroscope/v2/pkg/ingester/pyroscope"
	"github.com/grafana/pyroscope/v2/pkg/model/profileid"
	pprof2 "github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/util/tracecontext"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestIngest_OriginalTimestampProfileIDs(t *testing.T) {
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	compressed, err := os.ReadFile("../og/convert/jfr/testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz")
	require.NoError(t, err)
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	jfr, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("jfr", "profile.jfr")
	require.NoError(t, err)
	_, err = part.Write(jfr)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	speedscope, err := os.ReadFile("../og/convert/speedscope/testdata/simple.speedscope.json")
	require.NoError(t, err)
	profile, err := pprof2.RawFromBytes(collectTestProfileBytes(t))
	require.NoError(t, err)
	profile.TimeNanos = 0
	untimestamped, err := profile.MarshalVT()
	require.NoError(t, err)
	profile.TimeNanos = time.Now().UnixNano()
	timestamped, err := profile.MarshalVT()
	require.NoError(t, err)

	startTime := time.Now().UnixNano()
	from := strconv.FormatInt(startTime, 10)
	for _, input := range []struct {
		name        string
		format      string
		body        []byte
		contentType string
		embedded    bool
	}{
		{name: "jfr", format: "jfr", body: jfr},
		{name: "multipart jfr", format: "jfr", body: form.Bytes(), contentType: writer.FormDataContentType()},
		{name: "pprof without timestamp", format: "pprof", body: untimestamped},
		{name: "pprof with timestamp", format: "pprof", body: timestamped, embedded: true},
		{name: "lines", format: "lines", body: []byte("foo;bar 1\n")},
		{name: "groups", format: "groups", body: []byte("foo;bar 1\n")},
		{name: "speedscope", format: "speedscope", body: speedscope},
	} {
		for _, tt := range []struct {
			name     string
			from     string
			trace    bool
			disabled bool
			source   profileid.Source
		}{
			{name: "supplied timestamp", from: from, source: profileid.SourceTimestamp},
			{name: "timestamp before trace", from: from, trace: true, source: profileid.SourceTimestamp},
			{name: "missing timestamp with trace", trace: true, source: profileid.SourceTraceID},
			{name: "missing timestamp without trace", source: profileid.SourceRandom},
			{name: "zero timestamp with trace", from: "0", trace: true, source: profileid.SourceTraceID},
			{name: "zero timestamp without trace", from: "0", source: profileid.SourceRandom},
			{name: "disabled", from: from, trace: true, disabled: true, source: profileid.SourceRandom},
		} {
			t.Run(input.name+"/"+tt.name, func(t *testing.T) {
				limits := validation.MockOverrides(func(l *validation.Limits, _ map[string]*validation.Limits) {
					l.ProfileIDDeterministic = !tt.disabled
				})
				d, ing, err := newTestDistributor(t, log.NewNopLogger(), limits)
				require.NoError(t, err)
				h := tracecontext.HTTPMiddleware().Wrap(middleware.Tracer{}.Wrap(pyroscopeingest.NewPyroscopeIngestHandler(d, limits, log.NewNopLogger())))
				wantSource := tt.source
				if input.embedded && !tt.disabled {
					wantSource = profileid.SourceTimestamp
				}
				var firstIDs map[string]struct{}
				var firstCount float64
				for attempt := range 2 {
					start := len(ing.requests)
					ctx := tenant.InjectTenantID(t.Context(), "tenant")
					url := "/ingest?name=test&format=" + input.format
					if tt.from != "" {
						url += "&from=" + tt.from
					}
					req := httptest.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(input.body))
					req.Header.Set("Content-Type", input.contentType)
					if tt.trace {
						traceID := "0123456789abcdef0123456789abcdef"
						if attempt > 0 && wantSource == profileid.SourceTimestamp {
							traceID = "fedcba9876543210fedcba9876543210"
						}
						req.Header.Set("Traceparent", "00-"+traceID+"-0123456789abcdef-01")
					}
					resp := httptest.NewRecorder()
					h.ServeHTTP(resp, req)
					require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
					ids := make(map[string]struct{})
					for _, request := range ing.requests[start:] {
						for _, series := range request.Series {
							for _, sample := range series.Samples {
								ids[sample.ID] = struct{}{}
								stored, err := pprof2.RawFromBytes(sample.RawProfile)
								require.NoError(t, err)
								assert.Positive(t, stored.TimeNanos, "storage still receives a default timestamp")
								if input.embedded {
									assert.Equal(t, profile.TimeNanos, stored.TimeNanos)
								} else if tt.from == from {
									assert.Equal(t, startTime, stored.TimeNanos)
								}
							}
						}
					}
					require.NotEmpty(t, ids)
					count := testutil.ToFloat64(d.metrics.profileIDGeneration.WithLabelValues(string(wantSource)))
					if attempt == 0 {
						firstIDs, firstCount = ids, count
						if !tt.disabled {
							assert.Positive(t, count)
						}
					} else {
						if wantSource == profileid.SourceRandom {
							for id := range ids {
								assert.NotContains(t, firstIDs, id)
							}
						} else {
							assert.Equal(t, firstIDs, ids, "retries must preserve identity despite server defaults")
						}
						assert.Equal(t, 2*firstCount, count)
					}
				}
				for _, source := range []profileid.Source{profileid.SourceTimestamp, profileid.SourceTraceID, profileid.SourceRandom, profileid.SourceUserSupplied} {
					if source != wantSource || tt.disabled {
						assert.Zero(t, testutil.ToFloat64(d.metrics.profileIDGeneration.WithLabelValues(string(source))))
					}
				}
			})
		}
	}
}
