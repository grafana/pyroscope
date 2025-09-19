package integration

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gogo/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	profilesv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/og/convert/pprof/strprofile"
)

type otlpTestData struct {
	name             string
	profilePath      string
	expectedProfiles []expectedProfile
	assertMetrics    func(t *testing.T, p *PyroscopeTest)
}

type expectedProfile struct {
	metricName       string
	query            map[string]string
	expectedJsonPath string
}

var otlpTestDatas = []otlpTestData{
	{
		name:        "unsymbolized profile from otel-ebpf-profiler with cpu and offcpu",
		profilePath: "testdata/otel-ebpf-profile.pb.bin",
		expectedProfiles: []expectedProfile{
			{
				"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				map[string]string{"service_name": "unknown_service"},
				"testdata/otel-ebpf-profile.out.json",
			},
		},
		assertMetrics: func(t *testing.T, p *PyroscopeTest) {

		},
	},
}

func TestIngestOTLP(t *testing.T) {
	for _, td := range otlpTestDatas {
		t.Run(td.name, func(t *testing.T) {
			EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
				if td.profilePath != "testdata/otel-ebpf-profiler-unsymbolized.pb.bin" {
					t.Skip("Skipping OTLP ingestion integration tests")
				}

				rb := p.NewRequestBuilder(t)
				runNo := p.TempAppName()

				profileBytes, err := os.ReadFile(td.profilePath)
				require.NoError(t, err)
				var profile = new(profilesv1.ExportProfilesServiceRequest)
				err = proto.Unmarshal(profileBytes, profile)
				require.NoError(t, err)

				for _, rp := range profile.ResourceProfiles {
					for _, sp := range rp.ScopeProfiles {
						sp.Scope.Attributes = append(sp.Scope.Attributes, &commonv1.KeyValue{
							Key: "test_run_no", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: runNo}},
						})
					}
				}

				client := rb.OtelPushClient()
				_, err = client.Export(context.Background(), profile)
				require.NoError(t, err)

				for _, metric := range td.expectedProfiles {

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

					actual := strprofile.ToCompactProfile(resp.Msg, strprofile.Options{
						NoTime:     true,
						NoDuration: true,
					})
					strprofile.SortProfileSamples(actual)
					actualBytes, err := json.Marshal(actual)
					assert.NoError(t, err)

					pprofDumpFileName := strings.ReplaceAll(metric.expectedJsonPath, ".json", ".pprof.pb.bin") // for debugging
					pprof, err := resp.Msg.MarshalVT()
					assert.NoError(t, err)
					err = os.WriteFile(pprofDumpFileName, pprof, 0644)
					assert.NoError(t, err)

					expectedBytes, err := os.ReadFile(metric.expectedJsonPath)
					require.NoError(t, err)
					var expected strprofile.CompactProfile
					assert.NoError(t, json.Unmarshal(expectedBytes, &expected))
					strprofile.SortProfileSamples(expected)
					expectedBytes, err = json.Marshal(expected)
					require.NoError(t, err)

					assert.Equal(t, string(expectedBytes), string(actualBytes))
				}
				td.assertMetrics(t, p)
			})
		})
	}
}

type badOtlpTestData struct {
	name                 string
	profilePath          string
	expectedErrorMessage string
}

var badOtlpTestDatas = []badOtlpTestData{
	{
		name:                 "corrupted data (function idx out of bounds)",
		profilePath:          "testdata/otel-ebpf-profile-corrupted.pb.bin",
		expectedErrorMessage: "failed to convert otel profile: invalid stack index: 1000000000",
	},
}

func TestIngestBadOTLP(t *testing.T) {
	for _, td := range badOtlpTestDatas {
		t.Run(td.name, func(t *testing.T) {
			EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
				rb := p.NewRequestBuilder(t)
				profileBytes, err := os.ReadFile(td.profilePath)
				require.NoError(t, err)
				var profile = new(profilesv1.ExportProfilesServiceRequest)
				err = proto.Unmarshal(profileBytes, profile)
				require.NoError(t, err)

				client := rb.OtelPushClient()
				_, err = client.Export(context.Background(), profile)
				require.Error(t, err)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				if td.expectedErrorMessage != "" {
					require.Contains(t, err.Error(), td.expectedErrorMessage)
				}
			})
		})
	}
}
