package integration

import (
	"context"
	commonv1 "github.com/grafana/pyroscope/api/otlp/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
	"time"

	profilesv1 "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1development"
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
		name:        "t1",
		profilePath: "testdata/otel-ebpf-profiler-unsymbolized.pb.bin",
		expectedMetrics: []expectedProfile{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				map[string]string{"service_name": "unknown"},
				"testdata/otel-ebpf-profiler-unsymbolized.json",
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
			var profile profilesv1.ExportProfilesServiceRequest
			err = profile.Unmarshal(profileBytes)
			require.NoError(t, err)

			for _, rp := range profile.ResourceProfiles {
				for _, sp := range rp.ScopeProfiles {
					sp.Scope.Attributes = append(sp.Scope.Attributes, commonv1.KeyValue{
						Key: "test_run_no", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: runNo}},
					})
					for _, p := range sp.Profiles {
						p.DurationNanos = (time.Second * 10).Nanoseconds()
						p.TimeNanos = int64(time.Unix(10, 0).Nanosecond())
					}
				}
			}

			client := rb.OtelPushClient()
			_, err = client.Export(context.Background(), &profile)
			require.NoError(t, err)

			for _, metric := range td.expectedMetrics {

				expectedBytes, err := os.ReadFile(metric.expectedJsonPath)
				require.NoError(t, err)

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

				actualStr, err := strprofile.Stringify(resp.Msg, strprofile.Options{})

				pprofDumpFileName := strings.ReplaceAll(metric.expectedJsonPath, ".json", ".pprof.pb.bin")
				pprof, err := resp.Msg.MarshalVT()
				require.NoError(t, err)
				err = os.WriteFile(pprofDumpFileName, pprof, 0644)
				require.NoError(t, err)

				assert.JSONEq(t, string(expectedBytes), actualStr)
				_ = os.WriteFile(metric.expectedJsonPath, []byte(actualStr), 0644)
			}
		})
	}
}
