package pyroscope

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"sort"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	pprof2 "github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type flatProfileSeries struct {
	Labels     []*v1.LabelPair
	Profile    *profilev1.Profile
	RawProfile []byte
}

type MockPushService struct {
	Keep     bool
	reqPprof []*flatProfileSeries
	T        testing.TB
}

func (m *MockPushService) PushParsed(ctx context.Context, req *model.PushRequest) (*connect.Response[pushv1.PushResponse], error) {
	if m.Keep {
		for _, series := range req.Series {
			for _, sample := range series.Samples {
				rawProfileCopy := make([]byte, len(sample.RawProfile))
				copy(rawProfileCopy, sample.RawProfile)
				m.reqPprof = append(m.reqPprof, &flatProfileSeries{
					Labels:     series.Labels,
					Profile:    sample.Profile.Profile.CloneVT(),
					RawProfile: rawProfileCopy,
				})
			}
		}
	}
	return nil, nil
}

type DumpProfile struct {
	Collapsed  []string
	Labels     string
	SampleType string
}
type Dump struct {
	Profiles []DumpProfile
}

func (m *MockPushService) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	for _, series := range req.Msg.Series {
		for _, sample := range series.Samples {
			p, err := pprof.RawFromBytes(sample.RawProfile)
			if err != nil {
				return nil, err
			}
			m.reqPprof = append(m.reqPprof, &flatProfileSeries{
				Labels:  series.Labels,
				Profile: p.Profile.CloneVT(),
			})
		}
	}
	return nil, nil
}

func (m *MockPushService) selectActualProfile(ls labels.Labels, st string) DumpProfile {
	sort.Sort(ls)
	lss := ls.String()
	for _, p := range m.reqPprof {
		promLabels := phlaremodel.Labels(p.Labels).ToPrometheusLabels()
		sort.Sort(promLabels)
		actualLabels := labels.NewBuilder(promLabels).Del("jfr_event").Labels()
		als := actualLabels.String()
		if als == lss {
			for sti := range p.Profile.SampleType {
				actualST := p.Profile.StringTable[p.Profile.SampleType[sti].Type]
				if actualST == st {
					dp := DumpProfile{}
					dp.Labels = ls.String()
					dp.SampleType = actualST
					dp.Collapsed = bench.StackCollapseProto(p.Profile, sti, 1.0)
					slices.Sort(dp.Collapsed)
					return dp
				}
			}
		}
	}
	m.T.Fatalf("no profile found for %s %s", ls.String(), st)
	return DumpProfile{}
}

func (m *MockPushService) CompareDump(file string) {
	bs, err := bench.ReadGzipFile(file)
	require.NoError(m.T, err)

	expected := Dump{}
	err = json.Unmarshal(bs, &expected)
	require.NoError(m.T, err)

	var req []*flatProfileSeries
	for _, x := range m.reqPprof {
		iterateProfileSeries(x.Profile.CloneVT(), x.Labels, func(p *profilev1.Profile, ls phlaremodel.Labels) {
			req = append(req, &flatProfileSeries{
				Labels:  ls,
				Profile: p,
			})
		})
	}
	m.reqPprof = req

	for i := range expected.Profiles {
		expectedLabels := labels.Labels{}
		err := expectedLabels.UnmarshalJSON([]byte(expected.Profiles[i].Labels))
		require.NoError(m.T, err)

		actual := m.selectActualProfile(expectedLabels, expected.Profiles[i].SampleType)
		require.Equal(m.T, expected.Profiles[i].Collapsed, actual.Collapsed)
	}
}

const (
	repoRoot       = "../../../"
	testdataDirJFR = repoRoot + "pkg/og/convert/jfr/testdata"
)

func TestCorruptedJFR422(t *testing.T) {
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))

	src := testdataDirJFR + "/" + "cortex-dev-01__kafka-0__cpu__0.jfr.gz"
	jfr, err := bench.ReadGzipFile(src)
	require.NoError(t, err)

	jfr[0] = 0 // corrupt jfr

	svc := &MockPushService{Keep: true, T: t}
	h := NewPyroscopeIngestHandler(svc, l)

	res := httptest.NewRecorder()
	body, ct := createJFRRequestBody(t, jfr, nil)

	req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	h.ServeHTTP(res, req)

	require.Equal(t, 422, res.Code)
}

func createJFRRequestBody(t *testing.T, jfr, labels []byte) ([]byte, string) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	jfrw, err := w.CreateFormFile("jfr", "jfr")
	require.NoError(t, err)
	_, err = jfrw.Write(jfr)
	require.NoError(t, err)
	if labels != nil {
		labelsw, err := w.CreateFormFile("labels", "labels")
		require.NoError(t, err)
		_, err = labelsw.Write(labels)
		require.NoError(t, err)
	}
	err = w.Close()
	require.NoError(t, err)
	return b.Bytes(), w.FormDataContentType()
}

func BenchmarkIngestJFR(b *testing.B) {
	jfrs := []string{
		"cortex-dev-01__kafka-0__cpu__0.jfr.gz",
		"cortex-dev-01__kafka-0__cpu__1.jfr.gz",
		"cortex-dev-01__kafka-0__cpu__2.jfr.gz",
		"cortex-dev-01__kafka-0__cpu__3.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz",
	}
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	h := NewPyroscopeIngestHandler(&MockPushService{}, l)

	for _, jfr := range jfrs {
		b.Run(jfr, func(b *testing.B) {
			jfr, err := bench.ReadGzipFile(testdataDirJFR + "/" + jfr)
			require.NoError(b, err)
			for i := 0; i < b.N; i++ {
				res := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(jfr))
				req.Header.Set("Content-Type", "application/octet-stream")
				h.ServeHTTP(res, req)
			}
		})
	}
}

func TestIngestPPROFFixtures(t *testing.T) {
	testdata := []struct {
		profile          string
		prevProfile      string
		sampleTypeConfig string
		spyName          string

		expectStatus int
		expectMetric string
	}{
		{
			profile:      repoRoot + "pkg/pprof/testdata/heap",
			expectStatus: 200,
			expectMetric: "memory",
		},
		{
			profile:      repoRoot + "pkg/pprof/testdata/profile_java",
			expectStatus: 200,
			expectMetric: "process_cpu",
		},
		{
			profile:      repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatus: 200,
			expectMetric: "process_cpu",
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
			expectMetric: "process_cpu",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-exemplars.pb.gz",
			expectStatus: 200,
			expectMetric: "process_cpu",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-js.pb.gz",
			expectStatus: 200,
			expectMetric: "wall",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb",
			expectStatus: 200,
			expectMetric: "memory",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb.gz",
			expectStatus: 200,
			expectMetric: "memory",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap-js.pprof",
			expectStatus: 200,
			expectMetric: "memory",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-heap.pb.gz",
			expectStatus: 200,
			expectMetric: "memory",
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-wall.pb.gz",
			expectStatus: 200,
			expectMetric: "wall",
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_2.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_2.st.json",
			expectStatus:     200,
			expectMetric:     "goroutines",
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_3.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_3.st.json",
			expectStatus:     200,
			expectMetric:     "block",
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_4.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_4.st.json",
			expectStatus:     200,
			expectMetric:     "mutex",
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_5.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_5.st.json",
			expectStatus:     200,
			expectMetric:     "memory",
		},
		{
			// this one have milliseconds in Profile.TimeNanos
			// https://github.com/grafana/pyroscope/pull/2376/files
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/pyspy-1.pb.gz",
			expectStatus: 200,
			expectMetric: "process_cpu",
			spyName:      pprof2.SpyNameForFunctionNameRewrite(),
		},

		// todo add pprof from dotnet

	}
	for _, testdatum := range testdata {
		t.Run(testdatum.profile, func(t *testing.T) {
			var (
				profile, prevProfile, sampleTypeConfig []byte
				err                                    error
			)
			profile, err = os.ReadFile(testdatum.profile)
			require.NoError(t, err)
			if testdatum.prevProfile != "" {
				prevProfile, err = os.ReadFile(testdatum.prevProfile)
				require.NoError(t, err)
			}
			if testdatum.sampleTypeConfig != "" {
				sampleTypeConfig, err = os.ReadFile(testdatum.sampleTypeConfig)
				require.NoError(t, err)
			}

			bs, ct := createPProfRequest(t, profile, prevProfile, sampleTypeConfig)

			svc := &MockPushService{Keep: true, T: t}
			h := NewPyroscopeIngestHandler(svc, log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr)))

			res := httptest.NewRecorder()
			spyName := "foo239"
			if testdatum.spyName != "" {
				spyName = testdatum.spyName
			}
			req := httptest.NewRequest("POST", "/ingest?name=pprof.test{qwe=asd}&spyName="+spyName, bytes.NewReader(bs))
			req.Header.Set("Content-Type", ct)
			h.ServeHTTP(res, req)
			assert.Equal(t, testdatum.expectStatus, res.Code)

			if testdatum.expectStatus == 200 {
				require.Equal(t, 1, len(svc.reqPprof))
				actualReq := svc.reqPprof[0]
				ls := phlaremodel.Labels(actualReq.Labels)
				require.Equal(t, testdatum.expectMetric, ls.Get(labels.MetricName))
				require.Equal(t, "asd", ls.Get("qwe"))
				require.Equal(t, spyName, ls.Get(phlaremodel.LabelNamePyroscopeSpy))
				require.Equal(t, "pprof.test", ls.Get("service_name"))
				require.Equal(t, "false", ls.Get("__delta__"))
				require.Equal(t, profile, actualReq.RawProfile)

				if testdatum.spyName != pprof2.SpyNameForFunctionNameRewrite() {
					comparePPROF(t, actualReq.Profile, actualReq.RawProfile)
				}
			} else {
				assert.Equal(t, 0, len(svc.reqPprof))
			}
		})
	}
}

func comparePPROF(t *testing.T, actual *profilev1.Profile, profile2 []byte) {
	expected, err := pprof.RawFromBytes(profile2)
	require.NoError(t, err)

	require.Equal(t, len(expected.SampleType), len(actual.SampleType))
	for i := range actual.SampleType {
		require.Equal(t, expected.StringTable[expected.SampleType[i].Type], actual.StringTable[actual.SampleType[i].Type])
		require.Equal(t, expected.StringTable[expected.SampleType[i].Unit], actual.StringTable[actual.SampleType[i].Unit])

		actualCollapsed := bench.StackCollapseProto(actual, i, 1.0)
		expectedCollapsed := bench.StackCollapseProto(expected.Profile, i, 1.0)
		require.Equal(t, expectedCollapsed, actualCollapsed)
	}
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

func iterateProfileSeries(p *profilev1.Profile, seriesLabels phlaremodel.Labels, fn func(*profilev1.Profile, phlaremodel.Labels)) {
	for _, x := range p.Sample {
		sort.Sort(pprof.LabelsByKeyValue(x.Label))
	}
	sort.Sort(pprof.SamplesByLabels(p.Sample))
	groups := pprof.GroupSamplesWithoutLabels(p, "profile_id")
	e := pprof.NewSampleExporter(p)
	for _, g := range groups {
		ls := mergeSeriesAndSampleLabels(p, seriesLabels, g.Labels)
		ps := e.ExportSamples(new(profilev1.Profile), g.Samples)
		fn(ps, ls)
	}
}

func mergeSeriesAndSampleLabels(p *profilev1.Profile, sl []*v1.LabelPair, pl []*profilev1.Label) []*v1.LabelPair {
	m := phlaremodel.Labels(sl).Clone()
	for _, l := range pl {
		m = append(m, &v1.LabelPair{
			Name:  p.StringTable[l.Key],
			Value: p.StringTable[l.Str],
		})
	}
	sort.Stable(m)
	return m.Unique()
}
