package distributor

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"
)

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	d, err := New(Config{}, &mockRing{
		replicationFactor: 1,
		ingesters: []ring.InstanceDesc{
			{Addr: "foo"},
		},
	}, func(addr string) (client.PoolClient, error) {
		return newFakeIngester(t), nil
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
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}

type fakeIngester struct {
	t testing.TB
}

func (i *fakeIngester) Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
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

func newFakeIngester(t testing.TB) *fakeIngester {
	return &fakeIngester{t: t}
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
	if r.replicationFactor == 1 && len(r.ingesters) == 1 {
		result.MaxErrors = 0
		result.Instances = append(result.Instances, r.ingesters[0])
		return result, nil
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
