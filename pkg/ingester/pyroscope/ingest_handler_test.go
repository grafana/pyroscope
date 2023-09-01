package pyroscope

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
)

type MockPushService struct {
	Keep bool
	req  []*pushv1.RawProfileSeries
	T    testing.TB
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
	//fmt.Printf("pushing %d profiles\n", len(req.Msg.Series))
	if m.Keep {
		m.req = append(m.req, req.Msg.Series...)
	}
	res := &connect.Response[pushv1.PushResponse]{
		Msg: &pushv1.PushResponse{},
	}
	return res, nil
}

func (m *MockPushService) CompareDump(file string) {
	actual := m.Dump()
	bs, err := bench.ReadGzipFile(file)
	require.NoError(m.T, err)
	expected := Dump{}
	err = json.Unmarshal(bs, &expected)
	require.NoError(m.T, err)

	require.Equal(m.T, len(expected.Profiles), len(actual.Profiles))
	for i := range expected.Profiles {
		require.Equal(m.T, expected.Profiles[i].Labels, actual.Profiles[i].Labels)
		//if !reflect.DeepEqual(expected.Profiles[i].Collapsed, actual.Profiles[i].Collapsed) {
		//	os.WriteFile(file+"_expected.txt", []byte(strings.Join(expected.Profiles[i].Collapsed, "\n")), 0644)
		//	os.WriteFile(file+"_actual.txt", []byte(strings.Join(actual.Profiles[i].Collapsed, "\n")), 0644)
		//}
		require.Equal(m.T, expected.Profiles[i].Collapsed, actual.Profiles[i].Collapsed)
	}
}

func (m *MockPushService) DumpTo(file string) {
	d := m.Dump()
	bs, err := json.Marshal(d)
	require.NoError(m.T, err)
	err = bench.WriteGzipFile(file, bs)
	require.NoError(m.T, err)
}

func (m *MockPushService) Dump() Dump {
	res := Dump{}
	for _, series := range m.req {
		dp := DumpProfile{}
		dp.Labels = phlaremodel.Labels(series.Labels).ToPrometheusLabels().String()
		require.Equal(m.T, 1, len(series.Samples))
		p, err := profile.ParseData(series.Samples[0].RawProfile)
		require.NoError(m.T, err)
		slices.Sort(dp.Collapsed)
		require.Equal(m.T, 1, len(p.SampleType))
		dp.SampleType = p.SampleType[0].Type
		dp.Collapsed = bench.StackCollapseGoogle(p, 0)
		res.Profiles = append(res.Profiles, dp)

	}
	slices.SortFunc(res.Profiles, func(i, j DumpProfile) bool {
		labels := strings.Compare(i.Labels, j.Labels)
		if labels != 0 {
			return labels < 0
		}
		return strings.Compare(i.SampleType, j.SampleType) < 0
	})
	return res
}

func TestIngestJFR(b *testing.T) {
	testdataDir := "../../../pkg/og/convert/jfr/testdata"
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

	for _, jfr := range jfrs {
		b.Run(jfr, func(t *testing.T) {
			src := testdataDir + "/" + jfr
			jfr, err := bench.ReadGzipFile(src)
			svc := &MockPushService{Keep: true, T: t}
			h := NewPyroscopeIngestHandler(svc, l)
			require.NoError(t, err)

			res := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(jfr))
			req.Header.Set("Content-Type", "application/octet-stream")
			h.ServeHTTP(res, req)

			dst := strings.ReplaceAll(src, ".jfr.gz", ".pprof.json.gz")
			//svc.DumpTo(dst)
			svc.CompareDump(dst)

		})
	}

}

func BenchmarkIngestJFR(b *testing.B) {
	testdataDir := "../../../pkg/og/convert/jfr/testdata"
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
