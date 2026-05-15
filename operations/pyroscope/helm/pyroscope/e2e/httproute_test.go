package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

const (
	gatewayName    = "pyroscope"
	gatewayNS      = "default"
	gatewayHost    = "pyroscope.test"
	gatewayPort    = "8080"
	envoyGWNS      = "envoy-gateway-system"
	gwAPIVersion   = "v1.2.0"
	envoyGWVersion = "v1.2.0"
)

func init() {
	suiteSetup = setupHTTPRoute
}

// setupHTTPRoute installs the Gateway API CRDs, Envoy Gateway, the test
// GatewayClass/Gateway, and starts a port-forward to the Envoy proxy.
func setupHTTPRoute() (func(), error) {
	// ── 1. Gateway API CRDs ──────────────────────────────────────────────────
	fmt.Println("==> Installing Gateway API CRDs")
	if err := kubectl("apply", "-f",
		"https://github.com/kubernetes-sigs/gateway-api/releases/download/"+gwAPIVersion+"/standard-install.yaml",
	); err != nil {
		return nil, fmt.Errorf("gateway API CRDs: %w", err)
	}
	for _, crd := range []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"gatewayclasses.gateway.networking.k8s.io",
	} {
		if err := kubectl("wait", "--for=condition=established", "--timeout=1m", "crd/"+crd); err != nil {
			return nil, fmt.Errorf("wait for CRD %s: %w", crd, err)
		}
	}

	// ── 2. Envoy Gateway ─────────────────────────────────────────────────────
	fmt.Println("==> Installing Envoy Gateway", envoyGWVersion)
	if err := kubectl("apply", "--server-side", "-f",
		"https://github.com/envoyproxy/gateway/releases/download/"+envoyGWVersion+"/install.yaml",
	); err != nil {
		return nil, fmt.Errorf("envoy gateway: %w", err)
	}
	if err := kubectl("wait", "--timeout=5m",
		"-n", envoyGWNS,
		"deployment/envoy-gateway",
		"--for=condition=Available",
	); err != nil {
		return nil, fmt.Errorf("wait for envoy-gateway deployment: %w", err)
	}

	// ── 3. GatewayClass + Gateway ─────────────────────────────────────────────
	fmt.Println("==> Applying GatewayClass and Gateway")
	if err := kubectl("apply", "-f", filepath.Join(chartDir, "ci/gateway-resources.yaml")); err != nil {
		return nil, fmt.Errorf("gateway resources: %w", err)
	}
	if err := kubectl("wait", "gatewayclass/envoy", "--for=condition=Accepted", "--timeout=1m"); err != nil {
		return nil, fmt.Errorf("wait for GatewayClass: %w", err)
	}
	if err := kubectl("wait", "gateway/"+gatewayName,
		"-n", gatewayNS,
		"--for=condition=Programmed",
		"--timeout=2m",
	); err != nil {
		return nil, fmt.Errorf("wait for Gateway: %w", err)
	}

	// ── 4. Port-forward Envoy proxy ───────────────────────────────────────────
	fmt.Println("==> Port-forwarding Envoy proxy on :" + gatewayPort)
	pfCmd, err := startPortForward()
	if err != nil {
		return nil, fmt.Errorf("port-forward: %w", err)
	}
	return func() { _ = pfCmd.Process.Kill() }, nil
}

func startPortForward() (*exec.Cmd, error) {
	// Find the Envoy proxy service by label.
	out, err := exec.Command("kubectl",
		"--context", "kind-"+clusterName,
		"get", "svc", "-n", envoyGWNS,
		"-l", fmt.Sprintf(
			"gateway.envoyproxy.io/owning-gateway-name=%s,gateway.envoyproxy.io/owning-gateway-namespace=%s",
			gatewayName, gatewayNS,
		),
		"-o", "jsonpath={.items[0].metadata.name}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("get envoy svc: %w", err)
	}
	svcName := strings.TrimSpace(string(out))
	if svcName == "" {
		return nil, fmt.Errorf("envoy proxy service not found")
	}

	cmd := exec.Command("kubectl",
		"--context", "kind-"+clusterName,
		"port-forward",
		"-n", envoyGWNS,
		"svc/"+svcName,
		gatewayPort+":80",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start port-forward: %w", err)
	}

	// Wait until the port-forward is accepting connections.
	deadline := time.Now().Add(30 * time.Second)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://localhost:" + gatewayPort + "/")
		if err == nil {
			resp.Body.Close()
			return cmd, nil
		}
		time.Sleep(time.Second)
	}
	// Return the command even if not ready; the tests will produce clear errors.
	return cmd, nil
}

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
