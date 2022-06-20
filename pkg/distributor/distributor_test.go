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
	"testing"
	"time"

	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	ing := newFakeIngester(t, false)
	d, err := New(Config{}, &mockRing{
		replicationFactor: 3,
		ingesters: []ring.InstanceDesc{
			{Addr: "foo"},
		},
	}, func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	mux.Handle(pushv1connect.NewPusherHandler(d))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherClient(http.DefaultClient, s.URL)

	resp, err := client.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*pushv1.LabelPair{
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
				Labels: []*pushv1.LabelPair{
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
	d, err := New(Config{}, &mockRing{
		replicationFactor: 3,
		ingesters: []ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		},
	}, func(addr string) (client.PoolClient, error) {
		return ingesters[addr], nil
	}, log.NewLogfmtLogger(os.Stdout))
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
		PoolConfig: PoolConfig{ClientCleanupPeriod: 1 * time.Second},
	}, &mockRing{
		replicationFactor: 3,
		ingesters: []ring.InstanceDesc{
			{Addr: "foo"},
		},
	}, func(addr string) (client.PoolClient, error) {
		return ing, nil
	}, log.NewLogfmtLogger(os.Stdout))

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
}

func (i *fakeIngester) Push(_ context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	i.requests = append(i.requests, req.Msg)
	if i.fail {
		return nil, errors.New("foo")
	}
	res := connect.NewResponse(&pushv1.PushResponse{})
	return res, nil
}

func (i *fakeIngester) Close() error {
	return nil
}

func (i *fakeIngester) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

func (i *fakeIngester) Watch(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	return nil, errors.New("not implemented")
}

func newFakeIngester(t testing.TB, fail bool) *fakeIngester {
	return &fakeIngester{t: t, fail: fail}
}

type mockRing struct {
	ingesters         []ring.InstanceDesc
	replicationFactor uint32
}

func (r mockRing) Get(key uint32, op ring.Operation, buf []ring.InstanceDesc, _ []string, _ []string) (ring.ReplicationSet, error) {
	result := ring.ReplicationSet{
		MaxErrors: 1,
		Instances: buf[:0],
	}

	for i := uint32(0); i < r.replicationFactor; i++ {
		n := (key + i) % uint32(len(r.ingesters))
		result.Instances = append(result.Instances, r.ingesters[n])
	}
	return result, nil
}

func (r mockRing) GetAllHealthy(op ring.Operation) (ring.ReplicationSet, error) {
	return r.GetReplicationSetForOperation(op)
}

func (r mockRing) GetReplicationSetForOperation(op ring.Operation) (ring.ReplicationSet, error) {
	return ring.ReplicationSet{
		Instances: r.ingesters,
		MaxErrors: 1,
	}, nil
}

func (r mockRing) ReplicationFactor() int {
	return int(r.replicationFactor)
}

func (r mockRing) InstancesCount() int {
	return len(r.ingesters)
}

func (r mockRing) Subring(key uint32, n int) ring.ReadRing {
	return r
}

func (r mockRing) HasInstance(instanceID string) bool {
	for _, ing := range r.ingesters {
		if ing.Addr != instanceID {
			return true
		}
	}
	return false
}

func (r mockRing) ShuffleShard(identifier string, size int) ring.ReadRing {
	// take advantage of pass by value to bound to size:
	r.ingesters = r.ingesters[:size]
	return r
}

func (r mockRing) ShuffleShardWithLookback(identifier string, size int, lookbackPeriod time.Duration, now time.Time) ring.ReadRing {
	return r
}

func (r mockRing) CleanupShuffleShardCache(identifier string) {}

func (r mockRing) GetInstanceState(instanceID string) (ring.InstanceState, error) {
	return 0, nil
}
