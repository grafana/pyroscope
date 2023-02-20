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
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	"github.com/grafana/phlare/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/testhelper"
	"github.com/grafana/phlare/pkg/validation"
)

var ringConfig = RingConfig{
	KVStore:      kv.Config{Store: "inmemory"},
	InstanceID:   "foo",
	InstancePort: int(8080),
	InstanceAddr: "127.0.0.1",
	ListenPort:   int(8080),
}

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	mux.Handle(pushv1connect.NewPusherServiceHandler(d, connect.WithInterceptors(tenant.NewAuthInterceptor(true))))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherServiceClient(http.DefaultClient, s.URL, connect.WithInterceptors(tenant.NewAuthInterceptor(true)))

	resp, err := client.Push(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*typesv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
					{Name: "__name__", Value: "cpu"},
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
	ctx := tenant.InjectTenantID(context.Background(), "foo")
	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*typesv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
					{Name: "__name__", Value: "cpu"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testProfile(t),
					},
				},
			},
		},
	})
	d, err := New(Config{DistributorRing: ringConfig}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
		{Addr: "2"},
		{Addr: "3"},
	}, 3), func(addr string) (client.PoolClient, error) {
		return ingesters[addr], nil
	}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))
	require.NoError(t, err)
	// only 1 ingester failing should be fine.
	resp, err := d.Push(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	// 2 ingesters failing with a replication of 3 should return an error.
	ingesters["2"].fail = true
	resp, err = d.Push(ctx, req)
	require.Error(t, err)
	require.Nil(t, resp)
}

func Test_Subservices(t *testing.T) {
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		PoolConfig:      clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Second},
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 1), func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

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
	testhelper.FakePoolClient

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

func Test_Limits(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	mux.Handle(pushv1connect.NewPusherServiceHandler(d, connect.WithInterceptors(tenant.NewAuthInterceptor(true))))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherServiceClient(http.DefaultClient, s.URL, connect.WithInterceptors(tenant.NewAuthInterceptor(true)))

	t.Run("rate_limit", func(t *testing.T) {
		resp, err := client.Push(tenant.InjectTenantID(context.Background(), "user-1"), connect.NewRequest(&pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "cluster", Value: "us-central1"},
						{Name: "__name__", Value: "cpu"},
					},
					Samples: []*pushv1.RawSample{
						{
							RawProfile: testProfile(t),
						},
					},
				},
			},
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
		require.Nil(t, resp)
	})
	t.Run("label limit", func(t *testing.T) {
		resp, err := client.Push(tenant.InjectTenantID(context.Background(), "user-1"), connect.NewRequest(&pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "clusterdddwqdqdqdqdqdqw", Value: "us-central1"},
						{Name: "__name__", Value: "cpu"},
					},
					Samples: []*pushv1.RawSample{
						{
							RawProfile: testProfile(t),
						},
					},
				},
			},
		}))
		require.Error(t, err)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		require.Nil(t, resp)
	})
}

func newOverrides(t *testing.T) *validation.Overrides {
	t.Helper()
	return validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.IngestionRateMB = 0.0150
		l.IngestionBurstSizeMB = 0.0015
		l.MaxLabelNameLength = 10
		tenantLimits["user-1"] = l
	})
}
