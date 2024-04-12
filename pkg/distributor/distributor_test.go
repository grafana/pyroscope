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

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testhelper2 "github.com/grafana/pyroscope/pkg/pprof/testhelper"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	pprof2 "github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/validation"
)

var ringConfig = util.CommonRingConfig{
	KVStore:      kv.Config{Store: "inmemory"},
	InstanceID:   "foo",
	InstancePort: 8080,
	InstanceAddr: "127.0.0.1",
	ListenPort:   8080,
}

type poolFactory struct {
	f func(addr string) (client.PoolClient, error)
}

func (pf *poolFactory) FromInstance(inst ring.InstanceDesc) (client.PoolClient, error) {
	return pf.f(inst.Addr)
}

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), &poolFactory{func(addr string) (client.PoolClient, error) {
		return ing, nil
	}}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

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
						RawProfile: collectTestProfileBytes(t),
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
						RawProfile: collectTestProfileBytes(t),
					},
				},
			},
		},
	})
	d, err := New(Config{DistributorRing: ringConfig}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
		{Addr: "2"},
		{Addr: "3"},
	}, 3), &poolFactory{f: func(addr string) (client.PoolClient, error) {
		return ingesters[addr], nil
	}}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))
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
	}, 1), &poolFactory{f: func(addr string) (client.PoolClient, error) {
		return ing, nil
	}}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

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

func collectTestProfileBytes(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}

func hugeProfileBytes(t *testing.T) []byte {
	t.Helper()
	b := testhelper2.NewProfileBuilderWithLabels(time.Now().UnixNano(), nil)
	p := b.CPUProfile()
	for i := 0; i < 10_000; i++ {
		p.ForStacktraceString(fmt.Sprintf("my_%d", i), "other").AddSamples(1)
	}
	bs, err := p.Profile.MarshalVT()
	require.NoError(t, err)
	return bs
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
	type testCase struct {
		description              string
		pushReq                  *pushv1.PushRequest
		overrides                *validation.Overrides
		expectedCode             connect.Code
		expectedValidationReason validation.Reason
	}

	testCases := []testCase{
		{
			description: "rate_limit",
			pushReq: &pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "cluster", Value: "us-central1"},
							{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
							{Name: "__name__", Value: "cpu"},
						},
						Samples: []*pushv1.RawSample{
							{
								RawProfile: collectTestProfileBytes(t),
							},
						},
					},
				},
			},
			overrides: validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
				l := validation.MockDefaultLimits()
				l.IngestionRateMB = 0.0150
				l.IngestionBurstSizeMB = 0.0015
				tenantLimits["user-1"] = l
			}),
			expectedCode:             connect.CodeResourceExhausted,
			expectedValidationReason: validation.RateLimited,
		},
		{
			description: "rate_limit_invalid_profile",
			pushReq: &pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
							{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
						},
						Samples: []*pushv1.RawSample{{
							RawProfile: hugeProfileBytes(t),
						}},
					},
				},
			},
			overrides: validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
				l := validation.MockDefaultLimits()
				l.IngestionBurstSizeMB = 0.0015
				l.MaxProfileStacktraceSamples = 100
				tenantLimits["user-1"] = l
			}),
			expectedCode:             connect.CodeResourceExhausted,
			expectedValidationReason: validation.RateLimited,
		},
		{
			description: "labels_limit",
			pushReq: &pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "clusterdddwqdqdqdqdqdqw", Value: "us-central1"},
							{Name: "__name__", Value: "cpu"},
							{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
						},
						Samples: []*pushv1.RawSample{
							{
								RawProfile: collectTestProfileBytes(t),
							},
						},
					},
				},
			},
			overrides: validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
				l := validation.MockDefaultLimits()
				l.MaxLabelNameLength = 12
				tenantLimits["user-1"] = l
			}),
			expectedCode:             connect.CodeInvalidArgument,
			expectedValidationReason: validation.LabelNameTooLong,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			mux := http.NewServeMux()
			ing := newFakeIngester(t, false)
			d, err := New(Config{
				DistributorRing: ringConfig,
			}, testhelper.NewMockRing([]ring.InstanceDesc{
				{Addr: "foo"},
			}, 3), &poolFactory{f: func(addr string) (client.PoolClient, error) {
				return ing, nil
			}}, tc.overrides, nil, log.NewLogfmtLogger(os.Stdout))

			require.NoError(t, err)

			expectedMetricDelta := map[prometheus.Collector]float64{
				validation.DiscardedBytes.WithLabelValues(string(tc.expectedValidationReason), "user-1"): float64(uncompressedProfileSize(t, tc.pushReq)),
				// todo make sure pyroscope_distributor_received_decompressed_bytes_sum is not incremented
			}
			m1 := metricsDump(expectedMetricDelta)

			mux.Handle(pushv1connect.NewPusherServiceHandler(d, connect.WithInterceptors(tenant.NewAuthInterceptor(true))))
			s := httptest.NewServer(mux)
			defer s.Close()

			client := pushv1connect.NewPusherServiceClient(http.DefaultClient, s.URL, connect.WithInterceptors(tenant.NewAuthInterceptor(true)))
			resp, err := client.Push(tenant.InjectTenantID(context.Background(), "user-1"), connect.NewRequest(tc.pushReq))
			require.Error(t, err)
			require.Equal(t, tc.expectedCode, connect.CodeOf(err))
			require.Nil(t, resp)
			expectMetricsChange(t, m1, metricsDump(expectedMetricDelta), expectedMetricDelta)
		})
	}
}

func Test_Sessions_Limit(t *testing.T) {
	type testCase struct {
		description    string
		seriesLabels   phlaremodel.Labels
		expectedLabels phlaremodel.Labels
		maxSessions    int
	}

	testCases := []testCase{
		{
			description: "session_disabled",
			seriesLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: phlaremodel.LabelNameSessionID, Value: phlaremodel.SessionID(1).String()},
				{Name: "__name__", Value: "cpu"},
			},
			expectedLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "__name__", Value: "cpu"},
			},
			maxSessions: 0,
		},
		{
			description: "session_limited",
			seriesLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: phlaremodel.LabelNameSessionID, Value: phlaremodel.SessionID(4).String()},
				{Name: "__name__", Value: "cpu"},
			},
			expectedLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: phlaremodel.LabelNameSessionID, Value: phlaremodel.SessionID(1).String()},
				{Name: "__name__", Value: "cpu"},
			},
			maxSessions: 3,
		},
		{
			description: "session_not_specified",
			seriesLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "__name__", Value: "cpu"},
			},
			expectedLabels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "us-central1"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "__name__", Value: "cpu"},
			},
			maxSessions: 3,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			ing := newFakeIngester(t, false)
			d, err := New(
				Config{DistributorRing: ringConfig},
				testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
				&poolFactory{f: func(addr string) (client.PoolClient, error) { return ing, nil }},
				validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
					l := validation.MockDefaultLimits()
					l.MaxSessionsPerSeries = tc.maxSessions
					tenantLimits["user-1"] = l
				}), nil, log.NewLogfmtLogger(os.Stdout))

			require.NoError(t, err)
			assert.Equal(t, tc.expectedLabels, d.limitMaxSessionsPerSeries("user-1", tc.seriesLabels))
		})
	}
}

func Test_SampleLabels(t *testing.T) {
	type testCase struct {
		description string
		pushReq     *distributormodel.PushRequest
		series      []*distributormodel.ProfileSeries
	}

	testCases := []testCase{
		{
			description: "no series labels, no sample labels",
			pushReq: &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Samples: []*distributormodel.ProfileSample{
							{
								Profile: pprof2.RawFromProto(&profilev1.Profile{
									Sample: []*profilev1.Sample{{
										Value: []int64{1},
									}},
								}),
							},
						},
					},
				},
			},
			series: []*distributormodel.ProfileSeries{
				{
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								Sample: []*profilev1.Sample{{
									Value: []int64{1},
								}},
							}),
						},
					},
				},
			},
		},
		{
			description: "has series labels, no sample labels",
			pushReq: &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "foo", Value: "bar"},
						},
						Samples: []*distributormodel.ProfileSample{
							{
								Profile: pprof2.RawFromProto(&profilev1.Profile{
									Sample: []*profilev1.Sample{{
										Value: []int64{1},
									}},
								}),
							},
						},
					},
				},
			},
			series: []*distributormodel.ProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "foo", Value: "bar"},
					},
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								Sample: []*profilev1.Sample{{
									Value: []int64{1},
								}},
							}),
						},
					},
				},
			},
		},
		{
			description: "no series labels, all samples have identical label set",
			pushReq: &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Samples: []*distributormodel.ProfileSample{
							{
								Profile: pprof2.RawFromProto(&profilev1.Profile{
									StringTable: []string{"", "foo", "bar"},
									Sample: []*profilev1.Sample{{
										Value: []int64{1},
										Label: []*profilev1.Label{
											{Key: 1, Str: 2},
										},
									}},
								}),
							},
						},
					},
				},
			},
			series: []*distributormodel.ProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "foo", Value: "bar"},
					},
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								StringTable: []string{"", "foo", "bar"},
								Sample: []*profilev1.Sample{{
									Value: []int64{1},
									Label: []*profilev1.Label{},
								}},
							}),
						},
					},
				},
			},
		},
		{
			description: "has series labels, all samples have identical label set",
			pushReq: &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "baz", Value: "qux"},
						},
						Samples: []*distributormodel.ProfileSample{
							{
								Profile: pprof2.RawFromProto(&profilev1.Profile{
									StringTable: []string{"", "foo", "bar"},
									Sample: []*profilev1.Sample{{
										Value: []int64{1},
										Label: []*profilev1.Label{
											{Key: 1, Str: 2},
										},
									}},
								}),
							},
						},
					},
				},
			},
			series: []*distributormodel.ProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "foo", Value: "bar"},
					},
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								StringTable: []string{"", "foo", "bar"},
								Sample: []*profilev1.Sample{{
									Value: []int64{1},
									Label: []*profilev1.Label{},
								}},
							}),
						},
					},
				},
			},
		},
		{
			description: "has series labels, samples have distinct label sets",
			pushReq: &distributormodel.PushRequest{
				Series: []*distributormodel.ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "baz", Value: "qux"},
						},
						Samples: []*distributormodel.ProfileSample{
							{
								Profile: pprof2.RawFromProto(&profilev1.Profile{
									StringTable: []string{"", "foo", "bar", "waldo", "fred"},
									Sample: []*profilev1.Sample{
										{
											Value: []int64{1},
											Label: []*profilev1.Label{
												{Key: 1, Str: 2},
											},
										},
										{
											Value: []int64{2},
											Label: []*profilev1.Label{
												{Key: 3, Str: 4},
											},
										},
									},
								}),
							},
						},
					},
				},
			},
			series: []*distributormodel.ProfileSeries{
				{
					Labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "foo", Value: "bar"},
					},
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								StringTable: []string{""},
								Sample: []*profilev1.Sample{{
									Value: []int64{1},
									Label: []*profilev1.Label{},
								}},
							}),
						},
					},
				},
				{
					Labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "waldo", Value: "fred"},
					},
					Samples: []*distributormodel.ProfileSample{
						{
							Profile: pprof2.RawFromProto(&profilev1.Profile{
								StringTable: []string{""},
								Sample: []*profilev1.Sample{{
									Value: []int64{2},
									Label: []*profilev1.Label{},
								}},
							}),
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			series := extractSampleSeries(tc.pushReq)
			require.Len(t, series, len(tc.series))
			for i, actualSeries := range series {
				expectedSeries := tc.series[i]
				assert.Equal(t, expectedSeries.Labels, actualSeries.Labels)
				require.Len(t, actualSeries.Samples, len(expectedSeries.Samples))
				for j, actualProfile := range actualSeries.Samples {
					expectedProfile := expectedSeries.Samples[j]
					assert.Equal(t, expectedProfile.Profile.Sample, actualProfile.Profile.Sample)
				}
			}
		})
	}
}

func TestBadPushRequest(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), &poolFactory{f: func(addr string) (client.PoolClient, error) {
		return ing, nil
	}}, newOverrides(t), nil, log.NewLogfmtLogger(os.Stdout))

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
		&poolFactory{func(addr string) (client.PoolClient, error) {
			return ingesters[addr], nil
		}},
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

	// Empty profiles are discarded before sending to ingesters.
	var buf bytes.Buffer
	_, err = pprof2.RawFromProto(&profilev1.Profile{
		Sample: []*profilev1.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{1},
		}},
	}).WriteTo(&buf)
	require.NoError(t, err)
	profileBytes := buf.Bytes()

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
							{ID: "0000000-0000-0000-0000-000000000001", RawProfile: profileBytes},
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

func TestPush_Aggregation(t *testing.T) {
	const maxSessions = 8
	ingesterClient := newFakeIngester(t, false)
	d, err := New(
		Config{DistributorRing: ringConfig, PushTimeout: time.Second * 10},
		testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
		&poolFactory{f: func(addr string) (client.PoolClient, error) { return ingesterClient, nil }},
		validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
			l := validation.MockDefaultLimits()
			l.DistributorAggregationPeriod = model.Duration(time.Second)
			l.DistributorAggregationWindow = model.Duration(time.Second)
			l.MaxSessionsPerSeries = maxSessions
			tenantLimits["user-1"] = l
		}),
		nil, log.NewLogfmtLogger(os.Stdout),
	)
	require.NoError(t, err)
	ctx := tenant.InjectTenantID(context.Background(), "user-1")

	const (
		clients  = 10
		requests = 10
	)

	var wg sync.WaitGroup
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < requests; j++ {
				_, err := d.PushParsed(ctx, &distributormodel.PushRequest{
					Series: []*distributormodel.ProfileSeries{
						{
							Labels: []*typesv1.LabelPair{
								{Name: "cluster", Value: "us-central1"},
								{Name: "client", Value: strconv.Itoa(i)},
								{Name: "__name__", Value: "cpu"},
								{
									Name:  phlaremodel.LabelNameSessionID,
									Value: phlaremodel.SessionID(i*j + i).String(),
								},
							},
							Samples: []*distributormodel.ProfileSample{
								{
									Profile: &pprof2.Profile{
										Profile: testProfile(0),
									},
								},
							},
						},
					},
				})
				require.NoError(t, err)
			}
		}()
	}

	wg.Wait()
	d.asyncRequests.Wait()

	var sum int64
	sessions := make(map[string]struct{})
	assert.GreaterOrEqual(t, len(ingesterClient.requests), 20)
	assert.Less(t, len(ingesterClient.requests), 100)
	for _, r := range ingesterClient.requests {
		for _, s := range r.Series {
			sessionID := phlaremodel.Labels(s.Labels).Get(phlaremodel.LabelNameSessionID)
			sessions[sessionID] = struct{}{}
			p, err := pprof2.RawFromBytes(s.Samples[0].RawProfile)
			require.NoError(t, err)
			for _, x := range p.Sample {
				sum += x.Value[0]
			}
		}
	}

	// RF * samples_per_profile * clients * requests
	assert.Equal(t, int64(3*2*clients*requests), sum)
	assert.Equal(t, len(sessions), maxSessions)
}

func testProfile(t int64) *profilev1.Profile {
	return &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
				},
			},
			{
				LocationId: []uint64{1, 2, 3},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Line:      []*profilev1.Line{{FunctionId: 1}},
			},
			{
				Id:        2,
				MappingId: 1,
				Line:      []*profilev1.Line{{FunctionId: 2}},
			},
			{
				Id:        3,
				MappingId: 1,
				Line:      []*profilev1.Line{{FunctionId: 3}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       9,
				SystemName: 9,
				Filename:   10,
			},
			{
				Id:         2,
				Name:       11,
				SystemName: 11,
				Filename:   12,
			},
			{
				Id:         3,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
		},
		StringTable: []string{
			"",
			"samples",
			"count",
			"cpu",
			"nanoseconds",
			// Labels
			"foo",
			"bar",
			"function",
			"slow",
			// Functions
			"func-foo",
			"func-foo-path",
			"func-bar",
			"func-bar-path",
			"func-baz",
			"func-baz-path",
		},
		TimeNanos:     t,
		DurationNanos: 10000000000,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}
}

func TestInjectMappingVersions(t *testing.T) {
	alreadyVersionned := testProfile(3)
	alreadyVersionned.StringTable = append(alreadyVersionned.StringTable, `foo`)
	alreadyVersionned.Mapping[0].BuildId = int64(len(alreadyVersionned.StringTable) - 1)
	in := []*distributormodel.ProfileSeries{
		{
			Labels: []*typesv1.LabelPair{},
			Samples: []*distributormodel.ProfileSample{
				{
					Profile: &pprof2.Profile{
						Profile: testProfile(1),
					},
				},
			},
		},
		{
			Labels: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceRepository, Value: "grafana/pyroscope"},
			},
			Samples: []*distributormodel.ProfileSample{
				{
					Profile: &pprof2.Profile{
						Profile: testProfile(2),
					},
				},
			},
		},
		{
			Labels: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceRepository, Value: "grafana/pyroscope"},
				{Name: phlaremodel.LabelNameServiceGitRef, Value: "foobar"},
			},
			Samples: []*distributormodel.ProfileSample{
				{
					Profile: &pprof2.Profile{
						Profile: testProfile(2),
					},
				},
			},
		},
		{
			Labels: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceRepository, Value: "grafana/pyroscope"},
				{Name: phlaremodel.LabelNameServiceGitRef, Value: "foobar"},
			},
			Samples: []*distributormodel.ProfileSample{
				{
					Profile: &pprof2.Profile{
						Profile: alreadyVersionned,
					},
				},
			},
		},
	}

	err := injectMappingVersions(in)
	require.NoError(t, err)
	require.Equal(t, "", in[0].Samples[0].Profile.StringTable[in[0].Samples[0].Profile.Mapping[0].BuildId])
	require.Equal(t, `{"repository":"grafana/pyroscope"}`, in[1].Samples[0].Profile.StringTable[in[1].Samples[0].Profile.Mapping[0].BuildId])
	require.Equal(t, `{"repository":"grafana/pyroscope","git_ref":"foobar"}`, in[2].Samples[0].Profile.StringTable[in[2].Samples[0].Profile.Mapping[0].BuildId])
	require.Equal(t, `{"repository":"grafana/pyroscope","git_ref":"foobar","build_id":"foo"}`, in[3].Samples[0].Profile.StringTable[in[3].Samples[0].Profile.Mapping[0].BuildId])
}

func uncompressedProfileSize(t *testing.T, req *pushv1.PushRequest) int {
	var size int
	for _, s := range req.Series {
		for _, label := range s.Labels {
			size += len(label.Name) + len(label.Value)
		}
		for _, sample := range s.Samples {
			p, err := pprof2.RawFromBytes(sample.RawProfile)
			require.NoError(t, err)
			size += p.SizeVT()
		}
	}
	return size
}

func metricsDump(metrics map[prometheus.Collector]float64) map[prometheus.Collector]float64 {
	res := make(map[prometheus.Collector]float64)
	for m := range metrics {
		res[m] = testutil.ToFloat64(m)
	}
	return res
}

func expectMetricsChange(t *testing.T, m1, m2, expectedChange map[prometheus.Collector]float64) {
	for counter, expectedDelta := range expectedChange {
		delta := m2[counter] - m1[counter]
		assert.Equal(t, expectedDelta, delta, "metric %s", counter)
	}
}
