package pyroscope

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/grafana/pyroscope/pkg/pprof"

	"github.com/grafana/pyroscope/pkg/distributor/model"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
)

type flatProfileSeries struct {
	Labels  []*v1.LabelPair
	Profile *profilev1.Profile
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
				m.reqPprof = append(m.reqPprof, &flatProfileSeries{
					Labels:  series.Labels,
					Profile: sample.Profile.Profile,
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
				Profile: p.Profile,
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

	for i := range expected.Profiles {
		expectedLabels := labels.Labels{}
		err := expectedLabels.UnmarshalJSON([]byte(expected.Profiles[i].Labels))
		require.NoError(m.T, err)

		actual := m.selectActualProfile(expectedLabels, expected.Profiles[i].SampleType)
		require.Equal(m.T, expected.Profiles[i].Collapsed, actual.Collapsed)
	}
}

const testdataDir = "../../../pkg/og/convert/jfr/testdata"

func TestIngestJFR(b *testing.T) {
	testdata := []struct {
		jfr    string
		labels string
	}{
		{"cortex-dev-01__kafka-0__cpu__0.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu__1.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu__2.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu__3.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz", ""},
		{"cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz", ""},
		{"dump1.jfr.gz", "dump1.labels.pb.gz"},
		{"dump2.jfr.gz", "dump2.labels.pb.gz"},
	}
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))

	for _, jfr := range testdata {
		td := jfr
		b.Run(td.jfr, func(t *testing.T) {
			src := testdataDir + "/" + td.jfr
			jfr, err := bench.ReadGzipFile(src)
			require.NoError(t, err)
			var labels []byte
			if td.labels != "" {
				labels, err = bench.ReadGzipFile(testdataDir + "/" + td.labels)
			}
			require.NoError(t, err)
			svc := &MockPushService{Keep: true, T: t}
			h := NewPyroscopeIngestHandler(svc, l)

			res := httptest.NewRecorder()
			body, ct := createRequestBody(t, jfr, labels)

			req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(body))
			req.Header.Set("Content-Type", ct)
			h.ServeHTTP(res, req)

			dst := strings.ReplaceAll(src, ".jfr.gz", ".pprof.json.gz")
			//svc.DumpTo(dst)
			svc.CompareDump(dst)

		})
	}

}

func TestCorruptedJFR422(t *testing.T) {
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))

	src := testdataDir + "/" + "cortex-dev-01__kafka-0__cpu__0.jfr.gz"
	jfr, err := bench.ReadGzipFile(src)
	require.NoError(t, err)

	jfr[0] = 0 // corrupt jfr

	svc := &MockPushService{Keep: true, T: t}
	h := NewPyroscopeIngestHandler(svc, l)

	res := httptest.NewRecorder()
	body, ct := createRequestBody(t, jfr, nil)

	req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	h.ServeHTTP(res, req)

	require.Equal(t, 422, res.Code)
}

func createRequestBody(t *testing.T, jfr, labels []byte) ([]byte, string) {
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
			jfr, err := bench.ReadGzipFile(testdataDir + "/" + jfr)
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
