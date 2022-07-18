package distributor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"
	"github.com/grafana/fire/pkg/ingester/clientpool"
	"github.com/grafana/fire/pkg/testutil"
)

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{}, testutil.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, nil, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	mux.Handle(pushv1connect.NewPusherServiceHandler(d))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherServiceClient(http.DefaultClient, s.URL)

	resp, err := client.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*commonv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testProfile(t),
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 3, len(ing.requests[0].Series))
}

func Test_Replication(t *testing.T) {
	ingesters := map[string]*fakeIngester{
		"1": newFakeIngester(t, false),
		"2": newFakeIngester(t, false),
		"3": newFakeIngester(t, true),
	}
	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*commonv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testProfile(t),
					},
				},
			},
		},
	})
	d, err := New(Config{}, testutil.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
		{Addr: "2"},
		{Addr: "3"},
	}, 3), func(addr string) (client.PoolClient, error) {
		return ingesters[addr], nil
	}, nil, log.NewLogfmtLogger(os.Stdout))
	require.NoError(t, err)
	// only 1 ingester failing should be fine.
	resp, err := d.Push(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	// 2 ingesters failing with a replication of 3 should return an error.
	ingesters["2"].fail = true
	resp, err = d.Push(context.Background(), req)
	require.Error(t, err)
	require.Nil(t, resp)
}

func Test_Subservices(t *testing.T) {
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Second},
	}, testutil.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 1), func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, nil, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	require.NoError(t, d.StartAsync(context.Background()))
	require.Eventually(t, func() bool {
		fmt.Println(d.State())
		return d.State() == services.Running && d.pool.State() == services.Running
	}, 5*time.Second, 100*time.Millisecond)
	d.StopAsync()
	require.Eventually(t, func() bool {
		fmt.Println(d.State())
		return d.State() == services.Terminated && d.pool.State() == services.Terminated
	}, 5*time.Second, 100*time.Millisecond)
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}

type fakeIngester struct {
	t        testing.TB
	requests []*pushv1.PushRequest
	fail     bool
	testutil.FakePoolClient

	mtx sync.Mutex
}

func (i *fakeIngester) Push(_ context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.requests = append(i.requests, req.Msg)
	if i.fail {
		return nil, errors.New("foo")
	}
	res := connect.NewResponse(&pushv1.PushResponse{})
	return res, nil
}

func newFakeIngester(t testing.TB, fail bool) *fakeIngester {
	return &fakeIngester{t: t, fail: fail}
}

func TestBuckets(t *testing.T) {
	for _, r := range prometheus.ExponentialBucketsRange(10*1024, 15*1024*1024, 30) {
		t.Log(humanize.Bytes(uint64(r)))
	}
}

func TestSanitizeProfile(t *testing.T) {
	p := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 2, Unit: 1},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 3}, Value: []int64{0, 1}, Label: []*profilev1.Label{{Num: 10, Key: 1}, {Num: 11, Key: 1}}},
			// Those samples should be dropped.
			{LocationId: []uint64{1, 2, 3}, Value: []int64{0, 0}, Label: []*profilev1.Label{{Num: 10, Key: 1}}},
			{LocationId: []uint64{4}, Value: []int64{0, 0}, Label: []*profilev1.Label{{Num: 10, Key: 1}}},
		},
		Mapping: []*profilev1.Mapping{},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}, {FunctionId: 2, Line: 3}}},
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
			{Id: 4, Line: []*profilev1.Line{{FunctionId: 5, Line: 1}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 5, SystemName: 6, Filename: 7, StartLine: 1},
			{Id: 2, Name: 8, SystemName: 9, Filename: 10, StartLine: 1},
			{Id: 3, Name: 11, SystemName: 12, Filename: 13, StartLine: 1},
			{Id: 4, Name: 14, SystemName: 15, Filename: 7, StartLine: 1},
			{Id: 5, Name: 16, SystemName: 17, Filename: 18, StartLine: 1},
		},
		StringTable: []string{
			"memory", "bytes", "in_used", "allocs", "count",
			"main", "runtime.main", "main.go", // fn1
			"foo", "runtime.foo", "foo.go", // fn2
			"bar", "runtime.bar", "bar.go", // fn3
			"buzz", "runtime.buzz", // fn4
			"bla", "runtime.bla", "bla.go", // fn5
		},
		PeriodType:        &profilev1.ValueType{Type: 0, Unit: 1},
		Comment:           []int64{},
		DefaultSampleType: 0,
	}

	h := sanitizeProfile(p)
	require.Equal(t, h, &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 2, Unit: 1},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 3}, Value: []int64{0, 1}, Label: []*profilev1.Label{}},
		},
		Mapping: []*profilev1.Mapping{},
		Location: []*profilev1.Location{
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
		},
		Function: []*profilev1.Function{
			{Id: 2, Name: 6, SystemName: 7, Filename: 8, StartLine: 1},
			{Id: 3, Name: 9, SystemName: 10, Filename: 11, StartLine: 1},
			{Id: 4, Name: 12, SystemName: 13, Filename: 5, StartLine: 1},
		},
		StringTable: []string{
			"memory", "bytes", "in_used", "allocs", "count",
			"main.go",
			"foo", "runtime.foo", "foo.go",
			"bar", "runtime.bar", "bar.go",
			"buzz", "runtime.buzz",
		},
		PeriodType:        &profilev1.ValueType{Type: 0, Unit: 1},
		Comment:           []int64{},
		DefaultSampleType: 0,
	})
}
