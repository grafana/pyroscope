package e2e

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	clusterName   = "pyroscope-helm-e2e"
	kindNodeImage = "kindest/node:v1.33.7"
)

// chartDir is the path to the Helm chart root (the e2e/ package directory's parent).
// Tests are run with the working directory set to the package directory.
var chartDir = ".."

// suiteSetup is registered by a test suite's init() to run suite-specific
// infrastructure after the cluster is ready. It returns a cleanup function.
var suiteSetup func() (func(), error)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) (code int) {
	// ── 1. Kind cluster ──────────────────────────────────────────────────────
	fmt.Println("==> Creating Kind cluster", clusterName)
	if err := kubectl("cluster-info"); err != nil {
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

	// ── 2. Helm chart dependencies ────────────────────────────────────────────
	fmt.Println("==> Updating Helm chart dependencies")
	_ = helm("repo", "add", "minio", "https://charts.min.io/")
	_ = helm("repo", "add", "grafana", "https://grafana.github.io/helm-charts")
	if err := helm("dependency", "update", chartDir); err != nil {
		fmt.Fprintf(os.Stderr, "helm dependency update: %v\n", err)
		return 1
	}

	// ── 3. Suite-specific setup ───────────────────────────────────────────────
	if suiteSetup != nil {
		cleanup, err := suiteSetup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "suite setup: %v\n", err)
			return 1
		}
		defer cleanup()
	}

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
		"--timeout", "20m",
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
	return runCmd("helm", append([]string{"--kube-context", "kind-" + clusterName}, args...)...)
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

func randStr(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
