package integration

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

type speedscopeTestDataStruct struct {
	name            string
	speedscopeFile  string
	expectStatus    int
	expectedMetrics []expectedMetric
}

const (
	testdataDirSpeedscope = repoRoot + "pkg/og/convert/speedscope/testdata"
)

var (
	speedscopeTestData = []speedscopeTestDataStruct{
		{
			name:           "single profile evented",
			speedscopeFile: testdataDirSpeedscope + "/simple.speedscope.json",
			expectStatus:   200,
			expectedMetrics: []expectedMetric{
				// The difference between the metric name here and in the other test is a quirk in
				// how the speedscope parsing logic. Only multi profile uploads will
				// append the unit to the metric name which is parsed differently downstream.
				{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			},
		},
		{
			name:           "multi profile sampled",
			speedscopeFile: testdataDirSpeedscope + "/two-sampled.speedscope.json",
			expectStatus:   200,
			expectedMetrics: []expectedMetric{
				{"wall:wall:nanoseconds:cpu:nanoseconds", nil, 0},
			},
		},
		{
			name:           "multi profile samples bytes units",
			speedscopeFile: testdataDirSpeedscope + "/two-sampled-bytes.speedscope.json",
			expectStatus:   200,
			expectedMetrics: []expectedMetric{
				{"memory:samples:bytes::", nil, 0},
			},
		},
	}
)

func TestIngestSpeedscope(t *testing.T) {
	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
		for _, td := range speedscopeTestData {
			t.Run(td.name, func(t *testing.T) {
				rb := p.NewRequestBuilder(t)
				req := rb.IngestSpeedscopeRequest(td.speedscopeFile)
				p.Ingest(t, req, td.expectStatus)

				if td.expectStatus == 200 {
					for _, metric := range td.expectedMetrics {
						rb.Render(metric.name)
						profile := rb.SelectMergeProfile(metric.name, metric.query)
						assertSpeedscopeProfile(t, profile)
					}
				}
			})
		}
	})
}

func assertSpeedscopeProfile(t *testing.T, resp *connect.Response[profilev1.Profile]) {
	assert.Equal(t, 1, len(resp.Msg.SampleType), "SampleType should be set")
	require.Greater(t, len(resp.Msg.Sample), 0, "Profile should contain samples")
	assert.Greater(t, resp.Msg.Sample[0].Value[0], int64(0), "Sample value should be positive")
}
