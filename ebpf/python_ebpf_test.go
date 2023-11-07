package ebpfspy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/grafana/pyroscope/ebpf/testutil"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestEBPFPythonProfiler(t *testing.T) {
	var testdata = []struct {
		image    string
		expected string
	}{
		{"korniltsev/ebpf-testdata-rideshare:3.8-slim", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.9-slim", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.10-slim", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.11-slim", "python_ebpf_expected_3.11.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.12-slim", "python_ebpf_expected_3.11.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.13-rc-slim", "python_ebpf_expected_3.11.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.8-alpine", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.9-alpine", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.10-alpine", "python_ebpf_expected.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.11-alpine", "python_ebpf_expected_3.11.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.12-alpine", "python_ebpf_expected_3.11.txt"},
		{"korniltsev/ebpf-testdata-rideshare:3.13-rc-alpine", "python_ebpf_expected_3.11.txt"},
	}

	const ridesharePort = "5000"

	for _, testdatum := range testdata {
		testdatum := testdatum
		t.Run(testdatum.image, func(t *testing.T) {
			l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
			l = log.With(l, "test", t.Name())

			rideshare := testutil.RunContainerWithPort(t, l, testdatum.image, ridesharePort)
			defer rideshare.Kill()

			profiler := startPythonProfiler(t, l, rideshare.ContainerID)
			defer profiler.Stop()

			loadgen(t, l, rideshare.Url(), 2)

			profiles := collectProfiles(t, l, profiler)

			compareProfiles(t, l, testdatum.expected, profiles)
		})
	}

}

func compareProfiles(t *testing.T, l log.Logger, expected string, actual map[string]struct{}) {
	file, err := os.ReadFile(expected)
	require.NoError(t, err)
	expectedProfiles := map[string]struct{}{}
	for _, line := range strings.Split(string(file), "\n") {
		if line == "" {
			continue
		}
		expectedProfiles[line] = struct{}{}
		_ = l.Log("expected", line)
	}
	for line := range actual {
		_ = l.Log("actual", line)
	}

	for profile := range expectedProfiles {
		_, ok := actual[profile]
		require.True(t, ok, fmt.Sprintf("profile %s not found in actual", profile))
	}
}

func collectProfiles(t *testing.T, l log.Logger, profiler Session) map[string]struct{} {
	l = log.With(l, "component", "profiles")
	profiles := map[string]struct{}{}
	err := profiler.CollectProfiles(func(target *sd.Target, stack []string, value uint64, pid uint32) {
		lo.Reverse(stack)
		sample := strings.Join(stack, ";")
		profiles[sample] = struct{}{}
		_ = l.Log("target", target.String(),
			"pid", pid,
			"stack", sample)
	})
	require.NoError(t, err)
	return profiles
}

func startPythonProfiler(t *testing.T, l log.Logger, containerID string) Session {
	l = log.With(l, "component", "ebpf-session")
	targetFinder, err := sd.NewTargetFinder(os.DirFS("/"), l,
		sd.TargetsOptions{
			Targets: []sd.DiscoveryTarget{
				{
					"__container_id__": containerID,
					"service_name":     containerID,
				},
			},
			ContainerCacheSize: 1024,
			TargetsOnly:        true,
		})
	require.NoError(t, err)
	options := SessionOptions{
		CollectUser:   true,
		SampleRate:    97,
		Metrics:       metrics.New(nil),
		PythonEnabled: true,
		CacheOptions: symtab.CacheOptions{
			BuildIDCacheOptions: symtab.GCacheOptions{
				Size: 128, KeepRounds: 128,
			},
			SameFileCacheOptions: symtab.GCacheOptions{
				Size: 128, KeepRounds: 128,
			},
			PidCacheOptions: symtab.GCacheOptions{
				Size: 128, KeepRounds: 128,
			},
		},
	}
	session, err := NewSession(
		l,
		targetFinder,
		options,
	)
	require.NoError(t, err)

	err = session.Start()
	ci := os.Getenv("GITHUB_ACTIONS") == "true"
	_ = l.Log("err", err, "ci", ci, "msg", "session.Start")
	if ci {
		require.NoError(t, err)
	} else if err != nil {
		t.Skip("Skip because failed to start. Try running as privileged root user", err)
	}
	return session
}

func loadgen(t *testing.T, l log.Logger, url string, n int) {
	l = log.With(l, "component", "loadgen")
	orderVehicle := func(vehicle string) {
		url := fmt.Sprintf("%s/%s", url, vehicle)
		_ = l.Log("msg", "requesting", "url", url)
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = l.Log("msg", "response", "body", string(body))
	}
	for i := 0; i < n; i++ {
		orderVehicle("bike")
		orderVehicle("car")
		orderVehicle("scooter")
	}
}
