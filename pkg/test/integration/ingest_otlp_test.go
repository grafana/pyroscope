package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilesv1 "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1development"
	commonv1 "github.com/grafana/pyroscope/api/otlp/common/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/strprofile"
)

type otlpTestData struct {
	name            string
	profilePath     string
	expectedMetrics []expectedProfile
}

type expectedProfile struct {
	metricName       string
	query            map[string]string
	expectedJsonPath string
}

var otlpTestDatas = []otlpTestData{
	{
		name:        "unsymbolized profile from otel-ebpf-profiler",
		profilePath: "testdata/otel-ebpf-profiler-unsymbolized.pb.bin",
		expectedMetrics: []expectedProfile{
			{
				"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				map[string]string{"service_name": "unknown"},
				"testdata/otel-ebpf-profiler-unsymbolized.json",
			},
		},
	},
	{
		name:        "symbolized (with some help from pyroscope-ebpf profiler) profile from otel-ebpf-profiler",
		profilePath: "testdata/otel-ebpf-profiler-pyrosymbolized.pb.bin",
		expectedMetrics: []expectedProfile{
			{
				"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				map[string]string{"service_name": "unknown"},
				"testdata/otel-ebpf-profiler-pyrosymbolized-unknown.json",
			},
			{
				"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				map[string]string{"service_name": "otel-ebpf-docker//loving_robinson"},
				"testdata/otel-ebpf-profiler-pyrosymbolized-docker.json",
			},
		},
	},
}

func TestIngestOTLP(t *testing.T) {
	p := new(PyroscopeTest)
	p.Start(t)
	defer p.Stop(t)

	for _, td := range otlpTestDatas {
		t.Run(td.name, func(t *testing.T) {
			rb := p.NewRequestBuilder(t)
			runNo := p.TempAppName()

			profileBytes, err := os.ReadFile(td.profilePath)
			require.NoError(t, err)
			var profile = new(profilesv1.ExportProfilesServiceRequest)
			err = profile.Unmarshal(profileBytes)
			require.NoError(t, err)

			for _, rp := range profile.ResourceProfiles {
				for _, sp := range rp.ScopeProfiles {
					sp.Scope.Attributes = append(sp.Scope.Attributes, commonv1.KeyValue{
						Key: "test_run_no", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: runNo}},
					})
				}
			}

			client := rb.OtelPushClient()
			_, err = client.Export(context.Background(), profile)
			require.NoError(t, err)

			for _, metric := range td.expectedMetrics {

				expectedBytes, err := os.ReadFile(metric.expectedJsonPath)
				assert.NoError(t, err)

				query := make(map[string]string)
				for k, v := range metric.query {
					query[k] = v
				}
				query["test_run_no"] = runNo

				resp := rb.SelectMergeProfile(metric.metricName, query)

				assert.NotEmpty(t, resp.Msg.Sample)
				assert.NotEmpty(t, resp.Msg.Function)
				assert.NotEmpty(t, resp.Msg.Mapping)
				assert.NotEmpty(t, resp.Msg.Location)

				actualStr, err := strprofile.Stringify(resp.Msg, strprofile.Options{
					NoTime:     true,
					NoDuration: true,
				})
				assert.NoError(t, err)

				pprofDumpFileName := strings.ReplaceAll(metric.expectedJsonPath, ".json", ".pprof.pb.bin") // for debugging
				pprof, err := resp.Msg.MarshalVT()
				assert.NoError(t, err)
				err = os.WriteFile(pprofDumpFileName, pprof, 0644)
				assert.NoError(t, err)

				assert.Equal(t, string(expectedBytes), actualStr)
			}
		})
	}
}
