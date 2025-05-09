package integration

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/grafana/pyroscope/pkg/og/convert/pprof/strprofile"

	"github.com/grafana/pyroscope/pkg/pprof/testhelper"

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
			profile:            repoRoot + "pkg/pprof/testdata/heap_delta",
			expectStatusPush:   200,
			expectStatusIngest: 200,
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
	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
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
	})
}

func TestIngestPPROFFixPythonLinenumbers(t *testing.T) {
	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {

		profile := pprof.RawFromProto(&profilev1.Profile{
			SampleType: []*profilev1.ValueType{{
				Type: 5,
				Unit: 6,
			}},
			PeriodType: &profilev1.ValueType{
				Type: 5, Unit: 6,
			},
			StringTable: []string{"", "main", "func1", "func2", "qwe.py", "cpu", "nanoseconds"},
			Period:      1000000000,
			Function: []*profilev1.Function{
				{Id: 1, Name: 1, Filename: 4, SystemName: 1, StartLine: 239},
				{Id: 2, Name: 2, Filename: 4, SystemName: 2, StartLine: 42},
				{Id: 3, Name: 3, Filename: 4, SystemName: 3, StartLine: 7},
			},
			Location: []*profilev1.Location{
				{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 242}}},
				{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 50}}},
				{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 8}}},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{2, 1}, Value: []int64{10}},
				{LocationId: []uint64{3, 1}, Value: []int64{13}},
			},
		})

		tempProfileFile, err := os.CreateTemp("", "profile")
		require.NoError(t, err)
		_, err = profile.WriteTo(tempProfileFile)
		assert.NoError(t, err)
		tempProfileFile.Close()
		defer os.Remove(tempProfileFile.Name())

		rb := p.NewRequestBuilder(t).
			Spy("pyspy")
		req := rb.IngestPPROFRequest(tempProfileFile.Name(), "", "")
		p.Ingest(t, req, 200)

		renderedProfile := rb.SelectMergeProfile("process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil)
		actual := bench.StackCollapseProto(renderedProfile.Msg, 0, 1)
		expected := []string{
			"qwe.py main;qwe.py func1 10",
			"qwe.py main;qwe.py func2 13",
		}
		assert.Equal(t, expected, actual)
	})
}

func TestIngestPPROFSanitizeOtelLabels(t *testing.T) {
	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {

		p1 := testhelper.NewProfileBuilder(time.Now().Add(-time.Second).UnixNano()).
			CPUProfile().
			ForStacktraceString("my", "other").
			AddSamples(239)
		p1.Sample[0].Label = []*profilev1.Label{
			{
				Key: p1.AddString("foo.bar"),
				Str: p1.AddString("qwe-asd"),
			},
		}
		p1bs, err := p1.Profile.MarshalVT()
		require.NoError(t, err)

		rb := p.NewRequestBuilder(t)
		rb.Push(rb.PushPPROFRequestFromBytes(p1bs, "process_cpu"), 200, "")

		renderedProfile := rb.SelectMergeProfile("process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{
			"foo_bar": "qwe-asd",
		})
		actual, err := strprofile.Stringify(renderedProfile.Msg, strprofile.Options{
			NoTime:     true,
			NoDuration: true,
		})
		require.NoError(t, err)
		expected := `{
  "sample_types": [
    {
      "type": "cpu",
      "unit": "nanoseconds"
    }
  ],
  "samples": [
    {
      "locations": [
        {
          "address": "0x0",
          "lines": [
            "my[]@:0"
          ],
          "mapping": "0x0-0x0@0x0 ()"
        },
        {
          "address": "0x0",
          "lines": [
            "other[]@:0"
          ],
          "mapping": "0x0-0x0@0x0 ()"
        }
      ],
      "values": "239"
    }
  ],
  "period": "1000000000"
}`
		assert.JSONEq(t, expected, actual)
	})
}

func TestGodeltaprofRelabelPush(t *testing.T) {
	const blockSize = 1024
	const metric = "godeltaprof_memory"

	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {

		p1, _ := testhelper.NewProfileBuilder(time.Now().Add(-time.Second).UnixNano()).
			MemoryProfile().
			ForStacktraceString("my", "other").
			AddSamples(239, 239*blockSize, 1000, 1000*blockSize).
			Profile.MarshalVT()

		p2, _ := testhelper.NewProfileBuilder(time.Now().UnixNano()).
			MemoryProfile().
			ForStacktraceString("my", "other").
			AddSamples(3, 3*blockSize, 1000, 1000*blockSize).
			Profile.MarshalVT()

		rb := p.NewRequestBuilder(t)
		rb.Push(rb.PushPPROFRequestFromBytes(p1, metric), 200, "")
		rb.Push(rb.PushPPROFRequestFromBytes(p2, metric), 200, "")
		renderedProfile := rb.SelectMergeProfile("memory:alloc_objects:count:space:bytes", nil)
		actual := bench.StackCollapseProto(renderedProfile.Msg, 0, 1)
		expected := []string{
			"other;my 242",
		}
		assert.Equal(t, expected, actual)
	})
}

func TestPushStringTableOOBSampleType(t *testing.T) {
	const blockSize = 1024
	const metric = "godeltaprof_memory"

	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {

		testdata := []struct {
			name        string
			corrupt     func(p *testhelper.ProfileBuilder)
			expectedErr string
		}{
			{
				name: "sample type",
				corrupt: func(p *testhelper.ProfileBuilder) {
					p.SampleType[0].Type = 100500
				},
				expectedErr: "sample type type string index out of range",
			},
			{
				name: "function name",
				corrupt: func(p *testhelper.ProfileBuilder) {
					p.Function[0].Name = 100500
				},
				expectedErr: "function name string index out of range",
			},
			{
				name: "mapping",
				corrupt: func(p *testhelper.ProfileBuilder) {
					p.Mapping[0].Filename = 100500
				},
				expectedErr: "mapping file name string index out of range",
			},
			{
				name: "Sample label",
				corrupt: func(p *testhelper.ProfileBuilder) {
					p.Sample[0].Label = []*profilev1.Label{{
						Key: 100500,
					}}
				},
				expectedErr: "sample label string index out of range",
			},
			{
				name: "String 0 not empty",
				corrupt: func(p *testhelper.ProfileBuilder) {
					p.StringTable[0] = "hmmm"
				},
				expectedErr: "string 0 should be empty string",
			},
		}
		for _, td := range testdata {
			t.Run(td.name, func(t *testing.T) {
				p1 := testhelper.NewProfileBuilder(time.Now().Add(-time.Second).UnixNano()).
					MemoryProfile().
					ForStacktraceString("my", "other").
					AddSamples(239, 239*blockSize, 1000, 1000*blockSize)
				td.corrupt(p1)
				p1bs, err := p1.Profile.MarshalVT()
				require.NoError(t, err)

				rb := p.NewRequestBuilder(t)
				rb.Push(rb.PushPPROFRequestFromBytes(p1bs, metric), 400, td.expectedErr)
			})
		}
	})
}

func TestPush(t *testing.T) {
	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {

		for _, td := range testdata {
			if td.prevProfile != "" {
				continue
			}
			t.Run(td.profile, func(t *testing.T) {
				rb := p.NewRequestBuilder(t)

				req := rb.PushPPROFRequestFromFile(td.profile, td.metrics[0].name)
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
	})
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
