package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

// TestProfileDumpConnectCLI exercises real v2 ingestion and the operator workflow.
// Detailed encoding, validation, failure and retention boundaries live in package tests.
func TestProfileDumpConnectCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	now := time.Now().UTC()
	p := new(PyroscopeTest).Configure(t, true)
	p.config.MultitenancyEnabled = true
	p.config.Storage.Bucket.Prefix = "customer/prefix"
	bucketDir := p.config.Storage.Bucket.Filesystem.Directory
	runtimePath := filepath.Join(t.TempDir(), "runtime.yaml")
	require.NoError(t, os.WriteFile(runtimePath, []byte(fmt.Sprintf(`overrides:
  capture-test:
    profile_dump:
      active_until: %q
      probability: 1
      selector: '{service_name="checkout"}'
`, now.Add(5*time.Minute).Format(time.RFC3339Nano))), 0600))
	p.config.RuntimeConfig.LoadPath = []string{runtimePath}
	p.start(t)
	t.Cleanup(p.stop)
	client := pushv1connect.NewPusherServiceClient(http.DefaultClient, p.URL(), append(connectapi.DefaultClientOptions(), connect.WithInterceptors(tenant.NewAuthInterceptor(true)))...)
	rawBucket, err := filesystem.NewBucket(bucketDir)
	require.NoError(t, err)
	bucket := objstore.NewPrefixedBucket(rawBucket, "customer/prefix")
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	original, err := os.ReadFile(repoRoot + "pkg/pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	_, err = client.Push(tenant.InjectTenantID(ctx, "capture-test"), connect.NewRequest(&pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
		Labels:  []*typesv1.LabelPair{{Name: "__name__", Value: "cpu"}, {Name: "service_name", Value: "checkout"}},
		Samples: []*pushv1.RawSample{{ID: "compressed", RawProfile: original}},
	}}}))
	require.NoError(t, err)
	var key string
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var payloads []string
		err := bucket.Iter(ctx, profiledump.ObjectPrefix, func(name string) error {
			if strings.HasSuffix(name, ".pprof") {
				payloads = append(payloads, name)
			}
			return nil
		}, thanosobjstore.WithRecursiveIter())
		require.NoError(collect, err)
		require.Len(collect, payloads, 1)
		key = payloads[0]
		exists, err := bucket.Exists(ctx, strings.TrimSuffix(key, ".pprof")+".json")
		require.NoError(collect, err)
		require.True(collect, exists, "metadata sidecar is not stored yet")
	}, 10*time.Second, 10*time.Millisecond)

	cli := filepath.Join(t.TempDir(), "profilecli")
	build := exec.CommandContext(ctx, "go", "build", "-o", cli, "./cmd/profilecli")
	build.Dir = repoRoot
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	run := func(operation string, args ...string) ([]byte, error) {
		t.Helper()
		args = append([]string{"admin", "profile-dumps", operation, "--storage.backend=filesystem", "--storage.filesystem.dir=" + bucketDir, "--storage.prefix=customer/prefix"}, args...)
		command := exec.CommandContext(ctx, cli, args...)
		var out, diagnostic bytes.Buffer
		command.Stdout, command.Stderr = &out, &diagnostic
		err := command.Run()
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, diagnostic.String())
		}
		return out.Bytes(), nil
	}
	listed, err := run("list", "--tenant-id=capture-test", "--from="+now.Add(-time.Second).Format(time.RFC3339Nano), "--to="+now.Add(time.Minute).Format(time.RFC3339Nano))
	require.NoError(t, err)
	var result struct {
		Captures []struct {
			Key string `json:"key"`
		} `json:"captures"`
	}
	require.NoError(t, json.Unmarshal(listed, &result))
	require.Len(t, result.Captures, 1)
	require.Equal(t, key, result.Captures[0].Key)
	inspected, err := run("inspect", key)
	require.NoError(t, err)
	var info struct {
		Metadata profiledump.Metadata `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(inspected, &info))
	require.Equal(t, "capture-test", info.Metadata.TenantID)
	require.Equal(t, "compressed", info.Metadata.OriginalProfileID)
	path := filepath.Join(t.TempDir(), "capture.pprof")
	_, err = run("extract", key, "--output="+path)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, raw)
	sidecar, err := os.ReadFile(path + ".metadata.json")
	require.NoError(t, err)
	parsed, err := profiledump.ParseObjectKey(key)
	require.NoError(t, err)
	storedSidecar, err := os.ReadFile(filepath.Join(bucketDir, "customer/prefix", parsed.MetadataKey))
	require.NoError(t, err)
	require.Equal(t, storedSidecar, sidecar)

	// The cleaner must consume the writer's layout through the same bucket prefix.
	require.NoError(t, bucket.Upload(ctx, "unrelated/keep", strings.NewReader("keep")))
	cleanConfig := profiledump.CleanerConfig{Retention: 90 * time.Minute}
	registry := prometheus.NewRegistry()
	cleaner, err := profiledump.NewCleaner(cleanConfig, bucket, registry, func() time.Time { return now.Add(cleanConfig.Retention + time.Hour) })
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, cleaner))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), cleaner)) })
	require.Eventually(t, func() bool { return cleanupSweepSucceeded(registry) }, 5*time.Second, time.Millisecond)
	for _, sibling := range []string{parsed.PayloadKey, parsed.MetadataKey} {
		exists, err := bucket.Exists(ctx, sibling)
		require.NoError(t, err)
		require.False(t, exists)
	}
	exists, err := bucket.Exists(ctx, "unrelated/keep")
	require.NoError(t, err)
	require.True(t, exists)
}

func cleanupSweepSucceeded(reg *prometheus.Registry) bool {
	families, err := reg.Gather()
	if err != nil {
		return false
	}
	for _, family := range families {
		if family.GetName() == "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds" {
			return family.Metric[0].GetGauge().GetValue() > 0
		}
	}
	return false
}
