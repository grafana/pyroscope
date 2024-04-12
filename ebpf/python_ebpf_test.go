package ebpfspy

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/grafana/pyroscope/ebpf/testutil"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

//go:embed python_ebpf_expected.txt
var pythonEBPFExpected []byte

//go:embed python_ebpf_expected_3.11.txt
var pythonEBPFExpected311 []byte

func TestEBPFPythonProfiler(t *testing.T) {
	var testdata = []struct {
		image    string
		expected []byte
	}{
		{"korniltsev/ebpf-testdata-rideshare:3.8-slim", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.9-slim", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.10-slim", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.11-slim", pythonEBPFExpected311},
		{"korniltsev/ebpf-testdata-rideshare:3.12-slim", pythonEBPFExpected311},
		{"korniltsev/ebpf-testdata-rideshare:3.13-rc-slim", pythonEBPFExpected311},
		{"korniltsev/ebpf-testdata-rideshare:3.8-alpine", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.9-alpine", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.10-alpine", pythonEBPFExpected},
		{"korniltsev/ebpf-testdata-rideshare:3.11-alpine", pythonEBPFExpected311},
		{"korniltsev/ebpf-testdata-rideshare:3.12-alpine", pythonEBPFExpected311},
		{"korniltsev/ebpf-testdata-rideshare:3.13-rc-alpine", pythonEBPFExpected311},
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

func compareProfiles(t *testing.T, l log.Logger, expected []byte, actual map[string]struct{}) {
	expectedProfiles := map[string]struct{}{}
	for _, line := range strings.Split(string(expected), "\n") {
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
	err := profiler.CollectProfiles(func(ps pprof.ProfileSample) {
		lo.Reverse(ps.Stack)
		sample := strings.Join(ps.Stack, ";")
		profiles[sample] = struct{}{}
		_ = l.Log("target", ps.Target.String(),
			"pid", ps.Pid,
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
	s, err := NewSession(
		l,
		targetFinder,
		options,
	)
	require.NoError(t, err)

	err = s.Start()
	_ = l.Log("err", err, "msg", "session.Start")
	require.NoError(t, err, "Try running as privileged root user")

	impl := s.(*session)
	fake_target := sd.NewTargetForTesting(containerID, 0, map[string]string{
		"service_name": "fake",
	})
	perf := impl.getPyPerf(fake_target) // pyperf may take long time to load and verify, especially running in qemu with no kvm
	require.NotNil(t, perf)

	return s
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
