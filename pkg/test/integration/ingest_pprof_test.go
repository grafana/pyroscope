package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	pprof2 "github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
)

const repoRoot = "../../../"

var (
	golangHeap = []expectedMetric{
		{"memory:alloc_objects:count:space:bytes", 0},
		{"memory:alloc_space:bytes:space:bytes", 1},
		{"memory:inuse_objects:count:space:bytes", 2},
		{"memory:inuse_space:bytes:space:bytes", 3},
	}
	golangCPU = []expectedMetric{
		{"process_cpu:samples:count:cpu:nanoseconds", 0},
		{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", 1},
	}
	_        = golangHeap
	_        = golangCPU
	testdata = []pprofTestData{
		{
			profile:      repoRoot + "pkg/pprof/testdata/heap",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/pprof/testdata/profile_java",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", 0},
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			prevProfile:  repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatus: 422,
		},

		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu.pb.gz",
			prevProfile:  "",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-exemplars.pb.gz",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-js.pb.gz",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"wall:sample:count:wall:microseconds", 0},
				{"wall:wall:microseconds:wall:microseconds", 1},
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb.gz",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap-js.pprof",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"memory:space:bytes:space:bytes", 1},
				{"memory:objects:count:space:bytes", 0},
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-heap.pb.gz",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"memory:inuse_space:bytes:inuse_space:bytes", 1},
				{"memory:inuse_objects:count:inuse_space:bytes", 0},
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-wall.pb.gz",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"wall:samples:count:wall:microseconds", 0},
				{"wall:wall:microseconds:wall:microseconds", 1},
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_2.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_2.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"goroutines:goroutine:count:goroutine:count", 0},
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_3.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_3.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"block:delay:nanoseconds:contentions:count", 1},
				{"block:contentions:count:contentions:count", 0},
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_4.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_4.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"mutex:contentions:count:contentions:count", 0},
				{"mutex:delay:nanoseconds:contentions:count", 1},
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_5.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_5.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"memory:alloc_objects:count:space:bytes", 0},
				{"memory:alloc_space:bytes:space:bytes", 1},
			},
		},
		{
			// this one have milliseconds in Profile.TimeNanos
			// https://github.com/grafana/pyroscope/pull/2376/files
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/pyspy-1.pb.gz",
			expectStatus: 200,
			metrics: []expectedMetric{
				{"process_cpu:samples:count::milliseconds", 0},
			},
			spyName: pprof2.SpyNameForFunctionNameRewrite(),
		},

		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.pb.gz",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
			},
			needFunctionIDFix: true,
		},
		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			// it also has "-" in sample type name
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-73.pb.gz",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				// notice how they all use process_cpu metric
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
				{"process_cpu:alloc_samples:count::nanoseconds", 2}, // this was rewriten by ingest handler to replace -
				{"process_cpu:alloc_size:bytes::nanoseconds", 3},    // this was rewriten by ingest handler to replace -
			},
			needFunctionIDFix: true,
		},
		{
			// this is a fixed dotnet pprof
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-211.pb.gz",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-211.st.json",
			expectStatus:     200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
				{"process_cpu:alloc_samples:count::nanoseconds", 2},
				{"process_cpu:alloc_size:bytes::nanoseconds", 3},
			},
		},
	}
)

func TestIngest(t *testing.T) {
	p := PyroscopeTest{}
	p.Start(t)
	defer p.Stop(t)

	for _, testdatum := range testdata {
		t.Run(testdatum.profile, func(t *testing.T) {

			//todo do not only /ingest
			appName := ingest(t, testdatum)

			if testdatum.expectStatus == 200 {
				for _, metric := range testdatum.metrics {
					render(t, metric, appName, testdatum)
					selectMerge(t, metric, appName, testdatum)
				}
			}
		})
	}
}

func selectMerge(t *testing.T, metric expectedMetric, name string, testdatum pprofTestData) {
	qc := queryClient()
	resp, err := qc.SelectMergeProfile(context.Background(), connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: metric.name,
		Start:         time.Now().Add(-time.Hour).UnixMilli(),
		End:           time.Now().UnixMilli(),
		LabelSelector: fmt.Sprintf("{service_name=\"%s\"}", name),
	}))

	require.NoError(t, err)
	assert.Equal(t, 1, len(resp.Msg.SampleType))

	profileBytes, err := os.ReadFile(testdatum.profile)
	require.NoError(t, err)
	expectedProfile, err := pprof.RawFromBytes(profileBytes)
	require.NoError(t, err)

	actualStacktraces := bench.StackCollapseProto(resp.Msg, 0, 1)
	if testdatum.needFunctionIDFix {
		pprof2.FixFunctionIDForBrokenDotnet(expectedProfile.Profile)
	}
	expectedStacktraces := bench.StackCollapseProto(expectedProfile.Profile, metric.valueIDX, 1)

	for i, valueType := range expectedProfile.SampleType {
		fmt.Println(i, expectedProfile.StringTable[valueType.Type])
	}
	if testdatum.spyName == pprof2.SpyNameForFunctionNameRewrite() {
		fmt.Println("warning skipping scripting stacktrace check") // TODO
	} else {
		if !reflect.DeepEqual(expectedStacktraces, actualStacktraces) {
			name := filepath.Base(testdatum.profile)
			os.WriteFile(fmt.Sprintf("%s_%s_expected.txt", name, metric.name), []byte(strings.Join(expectedStacktraces, "\n")), 0666)
			os.WriteFile(fmt.Sprintf("%s_%s_actual.txt", name, metric.name), []byte(strings.Join(actualStacktraces, "\n")), 0666)
		}
		require.Equal(t, expectedStacktraces, actualStacktraces)
	}
}

func render(t *testing.T, metric expectedMetric, appName string, testdatum pprofTestData) {
	fmt.Println(metric)

	queryURL := "http://localhost:4040/pyroscope/render?query=" + metric.name + "{service_name=\"" + appName + "\"}&from=now-1h&until=now&format=collapsed"
	fmt.Println(queryURL)
	queryRes, err := http.Get(queryURL)
	require.NoError(t, err)
	body := bytes.NewBuffer(nil)
	_, err = io.Copy(body, queryRes.Body)
	assert.NoError(t, err)
	fb := new(flamebearer.FlamebearerProfile)
	err = json.Unmarshal(body.Bytes(), fb)
	assert.NoError(t, err, testdatum.profile, body.String(), queryURL)
	assert.Greater(t, len(fb.Flamebearer.Names), 1, testdatum.profile, body.String(), queryRes)
	assert.Greater(t, fb.Flamebearer.NumTicks, 1, testdatum.profile, body.String(), queryRes)
	// todo check actual stacktrace contents
}

type pprofTestData struct {
	profile           string
	prevProfile       string
	sampleTypeConfig  string
	spyName           string
	expectStatus      int
	metrics           []expectedMetric
	needFunctionIDFix bool
}

type expectedMetric struct {
	name     string
	valueIDX int
}

func ingest(t *testing.T, testdatum pprofTestData) string {
	var (
		profile, prevProfile, sampleTypeConfig []byte
		err                                    error
	)
	profile, err = os.ReadFile(testdatum.profile)
	assert.NoError(t, err)
	if testdatum.prevProfile != "" {
		prevProfile, err = os.ReadFile(testdatum.prevProfile)
		assert.NoError(t, err)
	}
	if testdatum.sampleTypeConfig != "" {
		sampleTypeConfig, err = os.ReadFile(testdatum.sampleTypeConfig)
		assert.NoError(t, err)
	}
	bs, ct := createPProfRequest(t, profile, prevProfile, sampleTypeConfig)

	spyName := "foo239"
	if testdatum.spyName != "" {
		spyName = testdatum.spyName
	}

	appName := fmt.Sprintf("pprof.integration.%s.%d",
		strings.ReplaceAll(filepath.Base(testdatum.profile), "-", "_"),
		rand.Uint64())
	url := "http://localhost:4040/ingest?name=" + appName + "&spyName=" + spyName
	req, err := http.NewRequest("POST", url, bytes.NewReader(bs))
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, testdatum.expectStatus, res.StatusCode, testdatum.profile)
	fmt.Printf("%+v %+v\n", testdatum, res)
	return appName
}

func createPProfRequest(t *testing.T, profile, prevProfile, sampleTypeConfig []byte) ([]byte, string) {
	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	profileW, err := w.CreateFormFile(formFieldProfile, "not used")
	require.NoError(t, err)
	_, err = profileW.Write(profile)
	require.NoError(t, err)

	if sampleTypeConfig != nil {

		sampleTypeConfigW, err := w.CreateFormFile(formFieldSampleTypeConfig, "not used")
		require.NoError(t, err)
		_, err = sampleTypeConfigW.Write(sampleTypeConfig)
		require.NoError(t, err)
	}

	if prevProfile != nil {
		prevProfileW, err := w.CreateFormFile(formFieldPreviousProfile, "not used")
		require.NoError(t, err)
		_, err = prevProfileW.Write(prevProfile)
		require.NoError(t, err)
	}
	err = w.Close()
	require.NoError(t, err)

	return b.Bytes(), w.FormDataContentType()
}

func queryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		http.DefaultClient,
		"http://localhost:4040",
	)
}
