package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/og/ingestion"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"

	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
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
			profile:            repoRoot + "pkg/pprof/testdata/heap",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
		},
		{
			profile:            repoRoot + "pkg/pprof/testdata/profile_java",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", 0},
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
				{"wall:sample:count:wall:microseconds", 0},
				{"wall:wall:microseconds:wall:microseconds", 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap.pb",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics:            golangHeap,
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/heap-js.pprof",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:space:bytes:space:bytes", 1},
				{"memory:objects:count:space:bytes", 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/nodejs-heap.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:inuse_space:bytes:inuse_space:bytes", 1},
				{"memory:inuse_objects:count:inuse_space:bytes", 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/nodejs-wall.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"wall:samples:count:wall:microseconds", 0},
				{"wall:wall:microseconds:wall:microseconds", 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_2.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_2.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"goroutines:goroutine:count:goroutine:count", 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_3.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_3.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"block:delay:nanoseconds:contentions:count", 1},
				{"block:contentions:count:contentions:count", 0},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_4.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_4.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"mutex:contentions:count:contentions:count", 0},
				{"mutex:delay:nanoseconds:contentions:count", 1},
			},
		},
		{
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/req_5.pprof",
			sampleTypeConfig:   repoRoot + "pkg/og/convert/pprof/testdata/req_5.st.json",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"memory:alloc_objects:count:space:bytes", 0},
				{"memory:alloc_space:bytes:space:bytes", 1},
			},
		},
		{
			// this one have milliseconds in Profile.TimeNanos
			// https://github.com/grafana/pyroscope/pull/2376/files
			profile:            repoRoot + "pkg/og/convert/pprof/testdata/pyspy-1.pb.gz",
			expectStatusIngest: 200,
			expectStatusPush:   200,
			metrics: []expectedMetric{
				{"process_cpu:samples:count::milliseconds", 0},
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
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
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
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
				{"process_cpu:alloc_samples:count::nanoseconds", 2}, // this was rewriten by ingest handler to replace -
				{"process_cpu:alloc_size:bytes::nanoseconds", 3},    // this was rewriten by ingest handler to replace -
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
				{"process_cpu:cpu:nanoseconds::nanoseconds", 0},
				{"process_cpu:alloc_samples:count::nanoseconds", 2},
				{"process_cpu:alloc_size:bytes::nanoseconds", 3},
				{"process_cpu:alloc_size:bytes::nanoseconds", 3},
			},
			spyName: "dotnetspy",
		},
	}
)

func TestIngest(t *testing.T) {
	p := PyroscopeTest{}
	p.Start(t)
	defer p.Stop(t)

	for _, testdatum := range testdata {
		t.Run(testdatum.profile, func(t *testing.T) {

			appName := ingest(t, testdatum)

			if testdatum.expectStatusIngest == 200 {
				for _, metric := range testdatum.metrics {
					render(t, metric, appName, testdatum)
					selectMerge(t, metric, appName, testdatum, true)
				}
			}
		})
	}
}

func TestPush(t *testing.T) {
	p := PyroscopeTest{}
	p.Start(t)
	defer p.Stop(t)

	for _, testdatum := range testdata {
		if testdatum.prevProfile != "" {
			continue
		}
		t.Run(testdatum.profile, func(t *testing.T) {

			appName := push(t, testdatum)

			if testdatum.expectStatusPush == 200 {
				for _, metric := range testdatum.metrics {
					render(t, metric, appName, testdatum)
					selectMerge(t, metric, appName, testdatum, false)
				}
			}
		})
	}
	//time.Sleep(10 * time.Hour)
}

func selectMerge(t *testing.T, metric expectedMetric, name string, testdatum pprofTestData, fixes bool) {
	qc := queryClient()
	resp, err := qc.SelectMergeProfile(context.Background(), connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: metric.name,
		Start:         time.Unix(0, 0).UnixMilli(),
		End:           time.Now().UnixMilli(),
		LabelSelector: fmt.Sprintf("{service_name=\"%s\"}", name),
	}))

	require.NoError(t, err)
	assert.Equal(t, 1, len(resp.Msg.SampleType))

	profileBytes, err := os.ReadFile(testdatum.profile)
	require.NoError(t, err)
	expectedProfile, err := pprof.RawFromBytes(profileBytes)
	require.NoError(t, err)

	if fixes {
		if testdatum.spyName == pprof2.SpyNameForFunctionNameRewrite() {
			pprof2.FixFunctionNamesForScriptingLanguages(expectedProfile, ingestion.Metadata{SpyName: testdatum.spyName})
		}

		if testdatum.needFunctionIDFix {
			pprof2.FixFunctionIDForBrokenDotnet(expectedProfile.Profile)
		}

	}
	actualStacktraces := bench.StackCollapseProto(resp.Msg, 0, 1)
	expectedStacktraces := bench.StackCollapseProto(expectedProfile.Profile, metric.valueIDX, 1)

	for i, valueType := range expectedProfile.SampleType {
		fmt.Println(i, expectedProfile.StringTable[valueType.Type])
	}
	require.Equal(t, expectedStacktraces, actualStacktraces)
}

func render(t *testing.T, metric expectedMetric, appName string, testdatum pprofTestData) {
	fmt.Println(metric)

	queryURL := "http://localhost:4040/pyroscope/render?query=" + metric.name + "{service_name=\"" + appName + "\"}&from=946656000&until=now&format=collapsed"
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
	profile            string
	prevProfile        string
	sampleTypeConfig   string
	spyName            string
	expectStatusIngest int
	expectStatusPush   int
	expectedError      string
	metrics            []expectedMetric
	needFunctionIDFix  bool
}

type expectedMetric struct {
	name     string
	valueIDX int
}

func push(t *testing.T, testdatum pprofTestData) string {
	appName := createAppname(testdatum)
	cl := pushClient()

	rawProfile, err := os.ReadFile(testdatum.profile)
	require.NoError(t, err)

	rawProfile = updateTimestamp(t, rawProfile)

	metricName := strings.Split(testdatum.metrics[0].name, ":")[0]

	_, err = cl.Push(context.TODO(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: metricName},
				{Name: "__delta__", Value: "false"},
				{Name: "service_name", Value: appName},
			},
			Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
		}},
	}))
	if testdatum.expectStatusPush == 200 {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
		var connectErr *connect.Error
		if ok := errors.As(err, &connectErr); ok {
			toHTTP := connectgrpc.CodeToHTTP(connectErr.Code())
			require.Equal(t, testdatum.expectStatusPush, int(toHTTP))
			if testdatum.expectedError != "" {
				require.Contains(t, connectErr.Error(), testdatum.expectedError)
			}
		} else {
			require.Fail(t, "unexpected error type", err.Error())
		}
	}

	return appName
}

func updateTimestamp(t *testing.T, rawProfile []byte) []byte {
	expectedProfile, err := pprof.RawFromBytes(rawProfile)
	require.NoError(t, err)
	expectedProfile.Profile.TimeNanos = time.Now().Add(-time.Minute).UnixNano()
	buf := bytes.NewBuffer(nil)
	_, err = expectedProfile.WriteTo(buf)
	require.NoError(t, err)
	rawProfile = buf.Bytes()
	return rawProfile
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

	appName := createAppname(testdatum)
	url := "http://localhost:4040/ingest?name=" + appName + "&spyName=" + spyName
	req, err := http.NewRequest("POST", url, bytes.NewReader(bs))
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, testdatum.expectStatusIngest, res.StatusCode, testdatum.profile)
	fmt.Printf("%+v %+v\n", testdatum, res)
	return appName
}

func createAppname(testdatum pprofTestData) string {
	return fmt.Sprintf("pprof.integration.%s.%d",
		strings.ReplaceAll(filepath.Base(testdatum.profile), "-", "_"),
		rand.Uint64())
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

func pushClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		http.DefaultClient,
		"http://localhost:4040",
	)
}
