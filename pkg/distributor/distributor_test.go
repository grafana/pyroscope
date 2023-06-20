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
	"strconv"
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

	phlaremodel "github.com/grafana/phlare/pkg/model"

	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	"github.com/grafana/phlare/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/clientpool"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/testhelper"
	"github.com/grafana/phlare/pkg/validation"
)

var ringConfig = RingConfig{
	KVStore:      kv.Config{Store: "inmemory"},
	InstanceID:   "foo",
	InstancePort: 8080,
	InstanceAddr: "127.0.0.1",
	ListenPort:   8080,
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
					{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
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
					{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
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
	for _, r := range prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount) {
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
						{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
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

func TestBadPushRequest(t *testing.T) {
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

	_, err = client.Push(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*typesv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
					{Name: "__name__", Value: "cpu"},
				},
			},
		},
	}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func newOverrides(t *testing.T) *validation.Overrides {
	t.Helper()
	return validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.IngestionRateMB = 0.0150
		l.IngestionBurstSizeMB = 0.0015
		l.MaxLabelNameLength = 12
		tenantLimits["user-1"] = l
	})
}

// this is a valid but pretty empty cpu pprof
func emptyCPUPprof() []byte {
	return []byte{0x1f, 0x8b, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xe2, 0x62, 0xe1, 0x60, 0x14, 0x60, 0xe2, 0x62, 0xe1, 0x60, 0x16, 0x60, 0xe1, 0x62, 0xe1, 0x60, 0x5, 0xb3, 0xd9, 0x4, 0x58, 0xa4, 0x4, 0x38, 0x18, 0x5, 0x1a, 0x1a, 0x1a, 0x98, 0x24, 0x1a, 0x1a, 0x76, 0xb, 0x69, 0xb0, 0x5b, 0x30, 0x1a, 0x31, 0x18, 0xf1, 0x26, 0xe6, 0xe4, 0xe4, 0x27, 0xc7, 0xe7, 0x27, 0x65, 0xa5, 0x26, 0x97, 0x14, 0x1b, 0xb1, 0x26, 0xe7, 0x97, 0xe6, 0x95, 0x18, 0x71, 0x43, 0x44, 0x8b, 0xb, 0x12, 0x93, 0x53, 0x8d, 0x58, 0x93, 0x2a, 0x4b, 0x52, 0x8b, 0x8d, 0x78, 0x33, 0xf3, 0x4a, 0x8b, 0x53, 0xe1, 0x2a, 0xb9, 0x21, 0x5c, 0x88, 0x12, 0x49, 0xfd, 0xd2, 0xe2, 0x22, 0xfd, 0x9c, 0xfc, 0xe4, 0xc4, 0x1c, 0xfd, 0xa4, 0xcc, 0x3c, 0xfd, 0x82, 0xa2, 0xfc, 0xdc, 0xd4, 0x92, 0x8c, 0xd4, 0xd2, 0x62, 0x23, 0x56, 0xb0, 0xa, 0x8f, 0x5f, 0x67, 0x3f, 0xfd, 0x9a, 0x73, 0xef, 0xfa, 0x4a, 0xf1, 0x80, 0x93, 0xad, 0x3d, 0x4f, 0x58, 0xa2, 0x58, 0x38, 0x38, 0x4, 0x58, 0x12, 0x1a, 0x1a, 0x14, 0x0, 0x1, 0x0, 0x0, 0xff, 0xff, 0x55, 0xb6, 0xc2, 0xa9, 0xae, 0x0, 0x0, 0x0}
}

func TestPush_ShuffleSharding(t *testing.T) {
	// initialize 10 fake ingesters
	var (
		ingesters = map[string]*fakeIngester{}
		ringDesc  = make([]ring.InstanceDesc, 10)
	)
	for pos := range ringDesc {
		ingesters[strconv.Itoa(pos)] = newFakeIngester(t, false)
		ringDesc[pos] = ring.InstanceDesc{
			Addr: strconv.Itoa(pos),
		}
	}

	overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		// 3 shards by default
		defaults.IngestionTenantShardSize = 3

		// user with sharding disabled
		user6 := validation.MockDefaultLimits()
		user6.IngestionTenantShardSize = 0
		tenantLimits["user-6"] = user6

		// user with only 1 shard (less than replication factor)
		user7 := validation.MockDefaultLimits()
		user7.IngestionTenantShardSize = 1
		tenantLimits["user-7"] = user7

		// user with 9 shards
		user8 := validation.MockDefaultLimits()
		user8.IngestionTenantShardSize = 9
		tenantLimits["user-8"] = user8

		// user with 27 shards (more shards than ingesters)
		user9 := validation.MockDefaultLimits()
		user9.IngestionTenantShardSize = 27
		tenantLimits["user-9"] = user9
	})

	// get distributor ready
	d, err := New(Config{DistributorRing: ringConfig}, testhelper.NewMockRing(ringDesc, 3),
		func(addr string) (client.PoolClient, error) {
			return ingesters[addr], nil
		},
		overrides,
		nil,
		log.NewLogfmtLogger(os.Stdout),
	)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.Handle(pushv1connect.NewPusherServiceHandler(d, connect.WithInterceptors(tenant.NewAuthInterceptor(true))))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherServiceClient(http.DefaultClient, s.URL, connect.WithInterceptors(tenant.NewAuthInterceptor(true)))

	for i := 0; i < 10; i++ {
		tenantID := fmt.Sprintf("user-%d", i)

		// push 50 series each
		for j := 0; j < 50; j++ {
			_, err = client.Push(tenant.InjectTenantID(context.Background(), tenantID), connect.NewRequest(&pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "pod", Value: fmt.Sprintf("my-stateful-stuff-%d", j)},
							{Name: "cluster", Value: "us-central1"},
							{Name: "tenant", Value: tenantID},
							{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
							{Name: "__name__", Value: "cpu"},
						},
						Samples: []*pushv1.RawSample{
							{ID: "0000000-0000-0000-0000-000000000001", RawProfile: emptyCPUPprof()},
						},
					},
				},
			}))
			require.NoError(t, err)
		}
	}

	ingestersByTenantID := make(map[string]map[string]int)

	// now let's check tenants per ingester
	for ingID, ing := range ingesters {
		ing.mtx.Lock()
		for _, req := range ing.requests {
			for _, s := range req.Series {
				for _, l := range s.Labels {
					if l.Name == "tenant" {
						m := ingestersByTenantID[l.Value]
						if m == nil {
							m = make(map[string]int)
							ingestersByTenantID[l.Value] = m
						}
						m[ingID]++
					}
				}
			}
		}
		ing.mtx.Unlock()
	}

	for tenantID, ingesters := range ingestersByTenantID {
		switch tenantID {
		case "user-6", "user-9": // users with disabled sharding and higher than ingester count should have all ingesters
			require.Equal(t, 10, len(ingesters))
		case "user-8": // user 8 has 9 configured
			require.Equal(t, 9, len(ingesters))
		default: // everyone else should fall back to 3, which is the replication factor
			require.Equal(t, 3, len(ingesters))

			var series int
			for _, count := range ingesters {
				series += count
			}
			require.Equal(t, 150, series)
		}
	}
}
