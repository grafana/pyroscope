//go:build integration

package distributor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/api"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	"github.com/grafana/pyroscope/v2/pkg/distributor"
	"github.com/grafana/pyroscope/v2/pkg/distributor/writepath"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// TestProfileDumpConnectCLI uses the registered API, real distributor parsing,
// recorder and filesystem provider. The ingestion destination is a fake ingester.
func TestProfileDumpConnectCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	now := time.Now().UTC()
	runtime, err := validation.LoadRuntimeConfigWithProfileDump(strings.NewReader(fmt.Sprintf(`overrides:
  capture-test:
    profile_debug_dump:
      active_until: %q
      probability: 1
      selector: '{service_name="checkout"}'
`, now.Add(time.Minute).Format(time.RFC3339Nano))), profiledump.DefaultConfig(), now)
	require.NoError(t, err)
	limits := validation.MockOverrides(func(defaults *validation.Limits, tenants map[string]*validation.Limits) {
		defaults.WritePathOverrides.WritePath = writepath.IngesterPath
		tenantLimits := *defaults
		tenantLimits.ProfileDebugDump = runtime.TenantLimits["capture-test"].ProfileDebugDump
		require.NoError(t, tenantLimits.Validate(profiledump.DefaultConfig(), now))
		tenants["capture-test"] = &tenantLimits
	})
	bucketDir := t.TempDir()
	rawBucket, err := filesystem.NewBucket(bucketDir)
	require.NoError(t, err)
	bucket := objstore.NewPrefixedBucket(rawBucket, "customer/prefix")
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	cfg := profiledump.DefaultRecorderConfig()
	var captureSeconds atomic.Int64
	recorder, err := profiledump.NewRecorder(cfg, profiledump.Dependencies{
		Now:      func() time.Time { return now.Add(time.Duration(captureSeconds.Load()) * time.Second) },
		Policies: limits, Upload: profiledump.BucketUpload(bucket, limits),
		DistributorID: "integration", Registerer: prometheus.NewRegistry(),
	})
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), recorder))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), recorder)) })
	d, err := distributor.NewTestDistributor(t, log.NewNopLogger(), limits)
	require.NoError(t, err)
	a := newAPITest(t, api.Config{}, log.NewNopLogger())
	a.RegisterDistributor(d, limits, server.Config{}, recorder)
	srv := httptest.NewServer(a.HTTPMux)
	t.Cleanup(srv.Close)
	client := pushv1connect.NewPusherServiceClient(srv.Client(), srv.URL, append(connectapi.DefaultClientOptions(), connect.WithInterceptors(tenant.NewAuthInterceptor(true)))...)
	original, err := os.ReadFile("../pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	malformed := []byte("malformed native pprof")
	push := func(raw []byte) error {
		_, err := client.Push(tenant.InjectTenantID(ctx, "capture-test"), connect.NewRequest(&pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
			Labels:  []*typesv1.LabelPair{{Name: "__name__", Value: "cpu"}, {Name: "service_name", Value: "checkout"}},
			Samples: []*pushv1.RawSample{{RawProfile: raw}},
		}}}))
		return err
	}
	require.NoError(t, push(original))
	captureSeconds.Add(1)
	require.Error(t, push(malformed), "normal ingestion must reject malformed pprof after capture")
	require.NoError(t, services.StopAndAwaitTerminated(ctx, recorder))
	var payloadKeys []string
	require.NoError(t, bucket.Iter(ctx, profiledump.NativeObjectPrefix, func(key string) error {
		if strings.HasSuffix(key, ".pprof") {
			payloadKeys = append(payloadKeys, key)
		}
		return nil
	}, thanosobjstore.WithRecursiveIter()))
	require.Len(t, payloadKeys, 2)
	t.Run("native", func(t *testing.T) {
		for _, key := range payloadKeys {
			keys, err := profiledump.ParseNativeObjectKey(key)
			require.NoError(t, err)
			reader, err := bucket.Get(ctx, key)
			require.NoError(t, err)
			payload, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			sidecar, err := bucket.Get(ctx, keys.MetadataKey)
			require.NoError(t, err)
			metadata, err := profiledump.ReadNativeMetadata(sidecar, keys.MetadataKey)
			require.NoError(t, err)
			require.NoError(t, sidecar.Close())
			require.Equal(t, int64(len(payload)), metadata.PayloadSize)
			require.Equal(t, "capture-test", metadata.TenantID)
			if metadata.PayloadSize == int64(len(original)) {
				require.Equal(t, original, payload)
				path := filepath.Join(t.TempDir(), "capture.pprof")
				require.NoError(t, os.WriteFile(path, payload, 0600))
				output, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", path).CombinedOutput()
				require.NoError(t, err, "%s", output)
				require.Contains(t, string(output), "flat")
				t.Logf("direct bucket download: %d identical bytes, go tool pprof succeeded\n%s", len(payload), output)
			} else {
				require.Equal(t, malformed, payload)
			}
		}
	})
	t.Run("cli_and_cleaner", func(t *testing.T) {
		cleanupTime := now
		cleanConfig := profiledump.DefaultCleanerConfig()
		unrelated := "unrelated/keep"
		outside, _, err := profiledump.NewNativeObjectKey("capture-test", now.Add(-8*24*time.Hour))
		require.NoError(t, err)
		require.NoError(t, rawBucket.Upload(ctx, outside, strings.NewReader("outside prefix")))
		require.NoError(t, bucket.Upload(ctx, unrelated, strings.NewReader("keep")))
		sweep := func() {
			registry := prometheus.NewRegistry()
			cleaner, err := profiledump.NewCleaner(cleanConfig, bucket, registry, func() time.Time { return cleanupTime })
			require.NoError(t, err)
			require.NoError(t, services.StartAndAwaitRunning(ctx, cleaner))
			require.Eventually(t, func() bool {
				return cleanupSweepSucceeded(registry)
			}, 5*time.Second, time.Millisecond)
			require.NoError(t, services.StopAndAwaitTerminated(ctx, cleaner))
		}
		sweep()
		cli := filepath.Join(t.TempDir(), "profilecli")
		build := exec.CommandContext(ctx, "go", "build", "-o", cli, "./cmd/profilecli")
		build.Dir = "../.."
		buildOutput, err := build.CombinedOutput()
		require.NoError(t, err, "%s", buildOutput)
		t.Run("cli", func(t *testing.T) {
			run := func(operation string, args ...string) []byte {
				t.Helper()
				args = append([]string{"admin", "profile-dumps", operation, "--storage.backend=filesystem", "--storage.filesystem.dir=" + bucketDir, "--storage.prefix=customer/prefix"}, args...)
				command := exec.CommandContext(ctx, cli, args...)
				var out, diagnostic bytes.Buffer
				command.Stdout, command.Stderr = &out, &diagnostic
				require.NoError(t, command.Run(), "%s", diagnostic.String())
				require.True(t, json.Valid(out.Bytes()), "%s", out.String())
				return out.Bytes()
			}
			var result struct {
				Captures []struct {
					Key string `json:"key"`
				} `json:"captures"`
			}
			listed := run("list", "--tenant-id=capture-test", "--from="+now.Add(-time.Second).Format(time.RFC3339Nano), "--to="+now.Add(time.Minute).Format(time.RFC3339Nano))
			require.NoError(t, json.Unmarshal(listed, &result))
			require.Len(t, result.Captures, 2)
			for _, capture := range result.Captures {
				var inspected struct {
					Metadata profiledump.NativeMetadata `json:"metadata"`
				}
				require.NoError(t, json.Unmarshal(run("inspect", capture.Key), &inspected))
				require.Equal(t, "capture-test", inspected.Metadata.TenantID)
				path := filepath.Join(t.TempDir(), "capture.pprof")
				extracted := run("extract", capture.Key, "--output="+path)
				require.Contains(t, string(extracted), "native-format validation")
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				sidecar, err := os.ReadFile(path + ".metadata.json")
				require.NoError(t, err)
				var metadata profiledump.NativeMetadata
				require.NoError(t, json.Unmarshal(sidecar, &metadata))
				require.Equal(t, inspected.Metadata, metadata)
				if metadata.PayloadSize == int64(len(original)) {
					require.Equal(t, original, raw)
					output, err := exec.CommandContext(ctx, "go", "tool", "pprof", "-top", path).CombinedOutput()
					require.NoError(t, err, "%s", output)
					require.Contains(t, string(output), "flat")
					t.Logf("valid fixture: %d identical bytes, go tool pprof -top succeeded\n%s", len(raw), output)
				} else {
					require.Equal(t, malformed, raw)
					t.Logf("malformed fixture: %d identical bytes extracted after ingestion rejection", len(raw))
				}
			}
		})
		t.Run("cleaner", func(t *testing.T) {
			cleanupTime = now.Add(cleanConfig.Retention + time.Hour)
			sweep()
			for _, key := range payloadKeys {
				exists, err := bucket.Exists(ctx, key)
				require.NoError(t, err)
				require.False(t, exists)
				parsed, err := profiledump.ParseNativeObjectKey(key)
				require.NoError(t, err)
				exists, err = bucket.Exists(ctx, parsed.MetadataKey)
				require.NoError(t, err)
				require.False(t, exists)
				args := []string{"admin", "profile-dumps", "inspect", "--storage.backend=filesystem", "--storage.filesystem.dir=" + bucketDir, "--storage.prefix=customer/prefix", key}
				output, err := exec.CommandContext(ctx, cli, args...).CombinedOutput()
				require.Error(t, err)
				require.Contains(t, string(output), "not found")
			}
			exists, err := bucket.Exists(ctx, unrelated)
			require.NoError(t, err)
			require.True(t, exists)
			outsideReader, err := rawBucket.Get(ctx, outside)
			require.NoError(t, err)
			outsideContents, err := io.ReadAll(outsideReader)
			closeErr := outsideReader.Close()
			require.NoError(t, err)
			require.NoError(t, closeErr)
			require.Equal(t, "outside prefix", string(outsideContents))
		})
	})
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
