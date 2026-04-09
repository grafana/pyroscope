package e2e

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	clusterName    = "pyroscope-httproute"
	gatewayName    = "pyroscope"
	gatewayNS      = "default"
	gatewayHost    = "pyroscope.test"
	gatewayPort    = "8080"
	envoyGWNS      = "envoy-gateway-system"
	gwAPIVersion   = "v1.2.0"
	envoyGWVersion = "v1.2.0"
	kindNodeImage  = "kindest/node:v1.33.7"
)

// chartDir is the path to the Helm chart root (the e2e/ package directory's parent).
// Tests are run with the working directory set to the package directory.
var chartDir = ".."

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) (code int) {
	// ── 1. Kind cluster ──────────────────────────────────────────────────────
	fmt.Println("==> Creating Kind cluster", clusterName)
	if err := kubectl("cluster-info", "--context", "kind-"+clusterName); err != nil {
		// Cluster doesn't exist yet, create it.
		if err := kind("create", "cluster",
			"--name", clusterName,
			"--image", kindNodeImage,
			"--wait", "5m",
		); err != nil {
			fmt.Fprintf(os.Stderr, "kind create cluster: %v\n", err)
			return 1
		}
	} else {
		fmt.Println("   (reusing existing cluster)")
	}
	defer func() {
		fmt.Println("==> Deleting Kind cluster", clusterName)
		_ = kind("delete", "cluster", "--name", clusterName)
	}()

	// ── 2. Gateway API CRDs ──────────────────────────────────────────────────
	fmt.Println("==> Installing Gateway API CRDs")
	if err := kubectl("apply", "-f",
		"https://github.com/kubernetes-sigs/gateway-api/releases/download/"+gwAPIVersion+"/standard-install.yaml",
	); err != nil {
		fmt.Fprintf(os.Stderr, "gateway API CRDs: %v\n", err)
		return 1
	}
	for _, crd := range []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"gatewayclasses.gateway.networking.k8s.io",
	} {
		if err := kubectl("wait", "--for=condition=established", "--timeout=60s", "crd/"+crd); err != nil {
			fmt.Fprintf(os.Stderr, "wait for CRD %s: %v\n", crd, err)
			return 1
		}
	}

	// ── 3. Envoy Gateway ─────────────────────────────────────────────────────
	fmt.Println("==> Installing Envoy Gateway", envoyGWVersion)
	if err := kubectl("apply", "--server-side", "-f",
		"https://github.com/envoyproxy/gateway/releases/download/"+envoyGWVersion+"/install.yaml",
	); err != nil {
		fmt.Fprintf(os.Stderr, "envoy gateway: %v\n", err)
		return 1
	}
	if err := kubectl("wait", "--timeout=5m",
		"-n", envoyGWNS,
		"deployment/envoy-gateway",
		"--for=condition=Available",
	); err != nil {
		fmt.Fprintf(os.Stderr, "wait for envoy-gateway deployment: %v\n", err)
		return 1
	}

	// ── 4. GatewayClass + Gateway ─────────────────────────────────────────────
	fmt.Println("==> Applying GatewayClass and Gateway")
	if err := kubectl("apply", "-f", filepath.Join(chartDir, "ci/gateway-resources.yaml")); err != nil {
		fmt.Fprintf(os.Stderr, "gateway resources: %v\n", err)
		return 1
	}
	if err := kubectl("wait", "gatewayclass/envoy", "--for=condition=Accepted", "--timeout=60s"); err != nil {
		fmt.Fprintf(os.Stderr, "wait for GatewayClass: %v\n", err)
		return 1
	}
	if err := kubectl("wait", "gateway/"+gatewayName,
		"-n", gatewayNS,
		"--for=condition=Programmed",
		"--timeout=120s",
	); err != nil {
		fmt.Fprintf(os.Stderr, "wait for Gateway: %v\n", err)
		return 1
	}

	// ── 5. Helm chart dependencies ────────────────────────────────────────────
	fmt.Println("==> Updating Helm chart dependencies")
	_ = helm("repo", "add", "minio", "https://charts.min.io/")
	_ = helm("repo", "add", "grafana", "https://grafana.github.io/helm-charts")
	if err := helm("dependency", "update", chartDir); err != nil {
		fmt.Fprintf(os.Stderr, "helm dependency update: %v\n", err)
		return 1
	}

	// ── 6. Port-forward Envoy proxy ───────────────────────────────────────────
	fmt.Println("==> Port-forwarding Envoy proxy on :" + gatewayPort)
	pfCmd, err := startPortForward()
	if err != nil {
		fmt.Fprintf(os.Stderr, "port-forward: %v\n", err)
		return 1
	}
	defer pfCmd.Process.Kill() //nolint:errcheck

	return m.Run()
}

// installChart installs the Helm chart with the given values file and returns
// the release name. It registers cleanup with t so the release is always removed.
func installChart(t *testing.T, valuesFile string) string {
	t.Helper()
	release := fmt.Sprintf("pyroscope-%s", randStr(8))
	ns := release

	t.Logf("helm install %s (values: %s)", release, valuesFile)
	if err := kubectl("create", "namespace", ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := helm("install", release, chartDir,
		"--namespace", ns,
		"--values", filepath.Join(chartDir, valuesFile),
		"--wait",
		"--timeout", "1200s",
	); err != nil {
		_ = kubectl("get", "pods", "-n", ns)
		t.Fatalf("helm install: %v", err)
	}
	t.Cleanup(func() {
		t.Logf("helm uninstall %s", release)
		_ = helm("uninstall", release, "--namespace", ns)
		_ = kubectl("delete", "namespace", ns, "--ignore-not-found")
	})
	return release
}

// ── helpers ────────────────────────────────────────────────────────────────────

func kind(args ...string) error {
	return runCmd("kind", args...)
}

func kubectl(args ...string) error {
	return runCmd("kubectl", append([]string{"--context", "kind-" + clusterName}, args...)...)
}

func helm(args ...string) error {
	return runCmd("helm", args...)
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, buf.String())
	}
	return nil
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

func randStr(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
