package integration

import (
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pprof2 "github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/pprof"
)

const repoRoot = "../../../"

var (
	golangHeap = []expectedMetric{
		{"memory:alloc_objects:count:space:bytes", nil, 0},
		{"memory:alloc_space:bytes:space:bytes", nil, 1},
		{"memory:inuse_objects:count:space:bytes", nil, 2},
		{"memory:inuse_space:bytes:space:bytes", nil, 3},
	}
	golangCPU = []expectedMetric{
		{"process_cpu:samples:count:cpu:nanoseconds", nil, 0},
		{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 1},
	}
	_        = golangHeap
	_        = golangCPU
	testdata = []pprofTestData{
		{
			profile:            repoRoot + "pkg/pprof/testdata/heap",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
			needsGoHeapFix:     true,
		},
		{
			profile:            repoRoot + "pkg/pprof/testdata/profile_java",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			},
		},
		{
			profile:            repoRoot + "pkg/pprof/testdata/go.cpu.labels.pprof",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangCPU,
		},
		{
			profile:            repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatusIngest: 200,
			expectStatusPush:   200,

			metrics: golangCPU,
		},
		{
			profile:            repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			prevProfile:        repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatusIngest: 422,
		},

		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/cpu.pb.gz",
			prevProfile:        "",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangCPU,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/cpu-exemplars.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangCPU,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/cpu-js.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"wall:sample:count:wall:microseconds", nil, 0},
				{"wall:wall:microseconds:wall:microseconds", nil, 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap.pb",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
			needsGoHeapFix:     true,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
			needsGoHeapFix:     true,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap-js.pprof",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:space:bytes:space:bytes", nil, 1},
				{"memory:objects:count:space:bytes", nil, 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/nodejs-heap.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:inuse_space:bytes:inuse_space:bytes", nil, 1},
				{"memory:inuse_objects:count:inuse_space:bytes", nil, 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/nodejs-wall.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"wall:samples:count:wall:microseconds", nil, 0},
				{"wall:wall:microseconds:wall:microseconds", nil, 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_2.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_2.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"goroutines:goroutine:count:goroutine:count", nil, 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_3.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_3.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"block:delay:nanoseconds:contentions:count", nil, 1},
				{"block:contentions:count:contentions:count", nil, 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_4.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_4.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"mutex:contentions:count:contentions:count", nil, 0},
				{"mutex:delay:nanoseconds:contentions:count", nil, 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_5.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_5.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:alloc_objects:count:space:bytes", nil, 0},
				{"memory:alloc_space:bytes:space:bytes", nil, 1},
			},
		},
		{
			// this one have milliseconds in Profile.TimeNanos
			// https://github.com/grafana/pyroscope/pull/2376/files
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/pyspy-1.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"process_cpu:samples:count::milliseconds", nil, 0},
			},
			spyName: pprof2.SpyNameForFunctionNameRewrite(),
		},
		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.pb.gz",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   400,
			expectedError:      "function id is 0",
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds::nanoseconds", nil, 0},
			},
			needFunctionIDFix: true,
			spyName:           "dotnetspy",
		},
		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			// it also has "-" in sample type name
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-73.pb.gz",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   400,
			expectedError:      "function id is 0",
			metrics: []expectedMetric{
				// notice how they all use process_cpu metric
				{"process_cpu:cpu:nanoseconds::nanoseconds", nil, 0},
				{"process_cpu:alloc_samples:count::nanoseconds", nil, 2}, // this was rewriten by ingest handler to replace -
				{"process_cpu:alloc_size:bytes::nanoseconds", nil, 3},    // this was rewriten by ingest handler to replace -
			},
			needFunctionIDFix: true,
			spyName:           "dotnetspy",
		},
		{
			// this is a fixed dotnet pprof
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-211.pb.gz",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-211.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds::nanoseconds", nil, 0},
				{"process_cpu:alloc_samples:count::nanoseconds", nil, 2},
				{"process_cpu:alloc_size:bytes::nanoseconds", nil, 3},
				{"process_cpu:alloc_size:bytes::nanoseconds", nil, 3},
			},
			spyName: "dotnetspy",
		},
		{

			profile:            repoRoot + "pkg/og/convert/pprof/testdata/invalid_utf8.pb.gz",
			expectStatusPush:   400,
			expectStatusIngest: 422,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds::nanoseconds", nil, 0},
			},
		},
	}
)

func TestIngest(t *testing.T) {
	p := PyroscopeTest{}
	p.Start(t)
	defer p.Stop(t)

	for _, td := range testdata {
		t.Run(td.profile, func(t *testing.T) {
			rb := p.NewRequestBuilder(t).
				Spy(td.spyName)
			req := rb.IngestPPROFRequest(td.profile, td.prevProfile, td.sampleTypeConfig)
			p.Ingest(t, req, td.expectStatusIngest)

			if td.expectStatusIngest == 200 {
				for _, metric := range td.metrics {
					rb.Render(metric.name)
					profile := rb.SelectMergeProfile(metric.name, metric.query)
					assertPPROF(t, profile, metric, td, td.fixAtIngest)
				}
			}
		})
	}
}

func TestPush(t *testing.T) {
	p := new(PyroscopeTest)
	p.Start(t)
	defer p.Stop(t)

	for _, td := range testdata {
		if td.prevProfile != "" {
			continue
		}
		t.Run(td.profile, func(t *testing.T) {
			rb := p.NewRequestBuilder(t)

			req := rb.PushPPROFRequest(td.profile, td.metrics[0].name)
			rb.Push(req, td.expectStatusPush, td.expectedError)

			if td.expectStatusPush == 200 {
				for _, metric := range td.metrics {
					rb.Render(metric.name)
					profile := rb.SelectMergeProfile(metric.name, metric.query)

					assertPPROF(t, profile, metric, td, td.fixAtPush)
				}
			}
		})
	}
}

func assertPPROF(t *testing.T, resp *connect.Response[profilev1.Profile], metric expectedMetric, testdatum pprofTestData, fix func(*pprof.Profile) *pprof.Profile) {

	assert.Equal(t, 1, len(resp.Msg.SampleType))

	profileBytes, err := os.ReadFile(testdatum.profile)
	require.NoError(t, err)
	expectedProfile, err := pprof.RawFromBytes(profileBytes)
	require.NoError(t, err)

	if fix != nil {
		expectedProfile = fix(expectedProfile)
	}

	actualStacktraces := bench.StackCollapseProto(resp.Msg, 0, 1)
	expectedStacktraces := bench.StackCollapseProto(expectedProfile.Profile, metric.valueIDX, 1)

	for i, valueType := range expectedProfile.SampleType {
		fmt.Println(i, expectedProfile.StringTable[valueType.Type])
	}
	require.Equal(t, expectedStacktraces, actualStacktraces)
}

type pprofTestData struct {
	profile            string
	prevProfile        string
	sampleTypeConfig   string
	spyName            string
	expectStatusIngest int
	expectStatusPush   int
	expectedError      string
	metrics            []expectedMetric
	needFunctionIDFix  bool
	needsGoHeapFix     bool
}

func (d *pprofTestData) fixAtPush(p *pprof.Profile) *pprof.Profile {
	if d.needsGoHeapFix {
		p.Profile = pprof.FixGoProfile(p.Profile)
	}
	return p
}

func (d *pprofTestData) fixAtIngest(p *pprof.Profile) *pprof.Profile {
	if d.spyName == pprof2.SpyNameForFunctionNameRewrite() {
		pprof2.FixFunctionNamesForScriptingLanguages(p, ingestion.Metadata{SpyName: d.spyName})
	}
	if d.needFunctionIDFix {
		pprof2.FixFunctionIDForBrokenDotnet(p.Profile)
	}
	if d.needsGoHeapFix {
		p.Profile = pprof.FixGoProfile(p.Profile)
	}
	return p
}

type expectedMetric struct {
	name     string
	query    map[string]string
	valueIDX int
}
