package integration

import (
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type jfrTestData struct {
	jfr    string
	labels string

	expectedMetrics []expectedMetric
	expectStatus    int
}

const (
	testdataDirJFR = repoRoot + "pkg/og/convert/jfr/testdata"
)

var jfrTestDatas = []jfrTestData{
	{
		jfr:             testdataDirJFR + "/cortex-dev-01__kafka-0__cpu__0.jfr.gz",
		expectStatus:    200,
		expectedMetrics: []expectedMetric{{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0}},
	},
	{
		jfr:             testdataDirJFR + "/cortex-dev-01__kafka-0__cpu__1.jfr.gz",
		expectStatus:    200,
		expectedMetrics: []expectedMetric{{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0}},
	},
	{
		jfr:             testdataDirJFR + "/cortex-dev-01__kafka-0__cpu__2.jfr.gz",
		expectStatus:    200,
		expectedMetrics: []expectedMetric{{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0}},
	},
	{
		jfr:             testdataDirJFR + "/cortex-dev-01__kafka-0__cpu__3.jfr.gz",
		expectStatus:    200,
		expectedMetrics: []expectedMetric{{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0}},
	},
	{
		jfr:          testdataDirJFR + "/cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", nil, 0},
		},
	},
	{
		jfr:          testdataDirJFR + "/cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", nil, 0},
		},
	},
	{
		jfr:          testdataDirJFR + "/cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", nil, 0},
		},
	},
	{
		jfr:          testdataDirJFR + "/cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", nil, 0},
		},
	},
	{
		jfr:          testdataDirJFR + "/cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", nil, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", nil, 0},
			{"memory:alloc_outside_tlab_objects:count:space:bytes", nil, 0},
			{"memory:alloc_outside_tlab_bytes:bytes:space:bytes", nil, 0},
			{"mutex:contentions:count:mutex:count", nil, 0},
			{"mutex:delay:nanoseconds:mutex:count", nil, 0},
			{"block:contentions:count:block:count", nil, 0},
			{"block:delay:nanoseconds:block:count", nil, 0},
		},
	},
	{
		jfr: testdataDirJFR + "/dump1.jfr.gz", labels: testdataDirJFR + "/dump1.labels.pb.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{

			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-7"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-3"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-5"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-1"}, 0},
		},
	},
	{
		jfr: testdataDirJFR + "/dump2.jfr.gz", labels: testdataDirJFR + "/dump2.labels.pb.gz",
		expectStatus: 200,
		expectedMetrics: []expectedMetric{
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-3"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-6"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-4"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-5"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-1"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-8"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-2"}, 0},
			{"wall:wall:nanoseconds:wall:nanoseconds", map[string]string{"thread_name": "pool-2-thread-7"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-3"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-6"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-4"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-5"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-1"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-8"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-2"}, 0},
			{"process_cpu:cpu:nanoseconds:cpu:nanoseconds", map[string]string{"thread_name": "pool-2-thread-7"}, 0},
			{"memory:alloc_in_new_tlab_objects:count:space:bytes", map[string]string{}, 0},
			{"memory:alloc_in_new_tlab_bytes:bytes:space:bytes", map[string]string{}, 0},
			{"memory:live:count:objects:count", map[string]string{}, 0},
		},
	},
}

const dump = false

func TestNOJFRDump(t *testing.T) {
	assert.False(t, dump)
}

func TestIngestJFR(t *testing.T) {
	p := new(PyroscopeTest)
	p.Start(t)
	defer p.Stop(t)

	for _, testdatum := range jfrTestDatas {
		t.Run(testdatum.jfr, func(t *testing.T) {

			rb := p.NewRequestBuilder(t)
			req := rb.IngestJFRRequestFiles(testdatum.jfr, testdatum.labels)
			p.Ingest(t, req, testdatum.expectStatus)

			if testdatum.expectStatus == 200 {
				assert.NotEqual(t, len(testdatum.expectedMetrics), 0)
				for _, metric := range testdatum.expectedMetrics {
					goldFile := expectedPPROFFile(testdatum, metric)
					t.Logf("%v gold file %s", metric, goldFile)
					rb.Render(metric.name)
					profile := rb.SelectMergeProfile(metric.name, metric.query)
					verifyPPROF(t, profile, goldFile, metric)
				}
			}
		})
	}
}

func expectedPPROFFile(testdatum jfrTestData, metric expectedMetric) string {
	lbls := phlaremodel.NewLabelsBuilder(nil)
	for k, v := range metric.query {
		lbls.Set(k, v)
	}
	ls := lbls.Labels()
	h := ls.Hash()

	return fmt.Sprintf("%s.%s.%d.pb.gz", testdatum.jfr, metric.name, h)
}

func TestCorruptedJFR422(t *testing.T) {
	p := new(PyroscopeTest)
	p.Start(t)
	defer p.Stop(t)

	td := jfrTestDatas[0]
	jfr, err := bench.ReadGzipFile(td.jfr)
	require.NoError(t, err)
	jfr[0] = 0 // corrupt jfr

	rb := p.NewRequestBuilder(t)
	req := rb.IngestJFRRequestBody(jfr, nil)
	p.Ingest(t, req, 422)
}

func verifyPPROF(t *testing.T, resp *connect.Response[profilev1.Profile], expectedPPROF string, metric expectedMetric) {
	var err error

	require.NoError(t, err)
	assert.Equal(t, 1, len(resp.Msg.SampleType))

	if dump {
		bs, err := resp.Msg.MarshalVT()
		require.NoError(t, err)
		err = bench.WriteGzipFile(expectedPPROF, bs)
		require.NoError(t, err)
	} else {
		profileBytes, err := os.ReadFile(expectedPPROF)
		require.NoError(t, err)
		expectedProfile, err := pprof.RawFromBytes(profileBytes)
		require.NoError(t, err)

		actualStacktraces := bench.StackCollapseProto(resp.Msg, 0, 1)
		expectedStacktraces := bench.StackCollapseProto(expectedProfile.Profile, metric.valueIDX, 1)

		require.Equal(t, expectedStacktraces, actualStacktraces)
	}
}
