package e2e

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	adhocprofilesv1 "github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1/adhocprofilesv1connect"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	querierv1connect "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// variants defines the chart configurations under test.
// Each installs the chart, runs all route assertions, then uninstalls.
var variants = []struct {
	name       string
	valuesFile string
}{
	{"single-binary", "ci/integration/httproute-values.yaml"},
	{"microservices", "ci/integration/httproute-microservices-values.yaml"},
}

// TestHTTPRoute installs the chart for each variant and verifies that all four
// HTTPRoute routing rules correctly deliver requests to Pyroscope.
func TestHTTPRoute(t *testing.T) {
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			installChart(t, v.valuesFile)

			httpClient := gatewayClient()
			baseURL := "http://localhost:" + gatewayPort
			ctx := context.Background()

			// ── query rule: / /querier.v1.QuerierService/ /render /render-diff ──
			t.Run("querier", func(t *testing.T) {
				client := querierv1connect.NewQuerierServiceClient(httpClient, baseURL)
				now := time.Now()
				_, err := client.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
					Start: now.Add(-time.Hour).UnixMilli(),
					End:   now.UnixMilli(),
				}))
				require.NoError(t, err)
			})

			// ── ingest rule: /push.v1.PusherService/ /ingest ──────────────────
			t.Run("pusher", func(t *testing.T) {
				client := pushv1connect.NewPusherServiceClient(httpClient, baseURL)
				_, err := client.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
					Series: []*pushv1.RawProfileSeries{{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "e2e.test.cpu.samples"},
							{Name: "service_name", Value: "e2e-test"},
						},
						Samples: []*pushv1.RawSample{{
							RawProfile: minimalPprofBytes(),
						}},
					}},
				}))
				require.NoError(t, err)
			})

			// ── settings rule: /settings.v1.SettingsService/ ─────────────────
			t.Run("settings", func(t *testing.T) {
				client := settingsv1connect.NewSettingsServiceClient(httpClient, baseURL)
				_, err := client.Get(ctx, connect.NewRequest(&settingsv1.GetSettingsRequest{}))
				require.NoError(t, err)
			})

			// ── adhoc profiles rule: /adhocprofiles.v1.AdHocProfileService/ ──
			t.Run("adhoc-profiles", func(t *testing.T) {
				client := adhocprofilesv1connect.NewAdHocProfileServiceClient(httpClient, baseURL)
				_, err := client.List(ctx, connect.NewRequest(&adhocprofilesv1.AdHocProfilesListRequest{}))
				require.NoError(t, err)
			})
		})
	}
}

// gatewayClient returns an HTTP client that routes through the Envoy Gateway
// by setting the Host and tenant headers on every request.
func gatewayClient() *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &gatewayTransport{base: http.DefaultTransport},
	}
}

type gatewayTransport struct {
	base http.RoundTripper
}

func (t *gatewayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Host = gatewayHost
	req.Header.Set("X-Scope-OrgID", "anonymous")
	return t.base.RoundTrip(req)
}

// minimalPprofBytes returns a minimal but valid gzip-compressed pprof profile
// with a single CPU sample, sufficient for routing tests.
func minimalPprofBytes() []byte {
	fn := &profile.Function{ID: 1, Name: "test.main", SystemName: "test.main", Filename: "test.go"}
	loc := &profile.Location{ID: 1, Line: []profile.Line{{Function: fn, Line: 1}}}
	p := &profile.Profile{
		SampleType:    []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:        10000000, // 10ms
		Function:      []*profile.Function{fn},
		Location:      []*profile.Location{loc},
		Sample:        []*profile.Sample{{Location: []*profile.Location{loc}, Value: []int64{1000000}}},
		TimeNanos:     time.Now().UnixNano(),
		DurationNanos: int64(time.Second),
	}
	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		panic("minimalPprofBytes: " + err.Error())
	}
	return buf.Bytes()
}
