package connectapi_test

import (
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/gorilla/mux"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/pyroscope/pkg/ingester"
	"github.com/grafana/pyroscope/pkg/phlare"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typev1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/distributor"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/validation"
)

type fakeTenantLimits struct {
	defaultLimits *validation.Limits
}

var compressedProfile []byte

func compressedProfileBytes(t testing.TB) []byte {
	if len(compressedProfile) == 0 {
		var err error
		compressedProfile, err = os.ReadFile("../../og/convert/pprof/testdata/cpu.pb.gz")
		if err != nil {
			t.Fatal(err)
		}
	}

	b := make([]byte, len(compressedProfile))
	copy(b, compressedProfile)
	return b
}

func (_ *fakeTenantLimits) AllByTenantID() map[string]*validation.Limits {
	panic("implement me")
}

func (f *fakeTenantLimits) TenantLimits(tenantID string) *validation.Limits {
	return f.defaultLimits
}

type poolFactory struct {
	f func(addr string) (client.PoolClient, error)
}

func (pf *poolFactory) FromInstance(inst ring.InstanceDesc) (client.PoolClient, error) {
	return pf.f(inst.Addr)
}

func newConfig(t testing.TB) *phlare.Config {
	cfg := &phlare.Config{}
	defaultFS := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlags(defaultFS)
	defaultFS.Parse([]string{})

	// no one needs ingestion windows
	cfg.LimitsConfig.IngestionRateMB = 2048
	cfg.LimitsConfig.RejectNewerThan = 0
	cfg.LimitsConfig.RejectOlderThan = 0

	cfg.Distributor.DistributorRing = util.CommonRingConfig{
		KVStore:      kv.Config{Store: "inmemory"},
		InstanceID:   "foo",
		InstancePort: 8080,
		InstanceAddr: "127.0.0.1",
		ListenPort:   8080,
	}
	return cfg
}

type fakeIngester struct {
	*ingester.Ingester
	bytesReceived atomic.Uint64
}

func (i *fakeIngester) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	for _, series := range req.Msg.Series {
		for _, sample := range series.Samples {
			err := pprof.FromBytes(sample.RawProfile, func(_ *profilev1.Profile, bytes int) error {
				i.bytesReceived.Add(uint64(bytes))
				return nil
			})
			if err != nil {
				return nil, err
			}

		}
	}
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

type poolClient struct {
	pushv1connect.PusherServiceClient
}

func (p poolClient) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (p poolClient) Watch(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	//TODO implement me
	panic("implement me")
}

func (p poolClient) Close() error {
	//TODO implement me
	panic("implement me")
}

type tenantRoundTripper struct {
	upstream  http.RoundTripper
	tenantID  string
	bytesRead atomic.Uint64
	bytesSent atomic.Uint64
}

type readerCount struct {
	io.ReadCloser
	bytes int
}

func (r *readerCount) Read(p []byte) (n int, err error) {
	n, err = r.ReadCloser.Read(p)
	r.bytes += n
	return n, err
}

func (tr *tenantRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	rc := &readerCount{r.Body, 0}
	r.Body = rc
	if tr.tenantID != "" {
		r.Header.Set("X-Scope-OrgID", tr.tenantID)
	}
	resp, err := tr.upstream.RoundTrip(r)
	tr.bytesSent.Add(uint64(resp.ContentLength))
	tr.bytesRead.Add(uint64(rc.bytes))
	return resp, err
}

type ingestBenchmark struct {
	handlerOptions []connect.HandlerOption
	clientOptions  []connect.ClientOption

	clientD pushv1connect.PusherServiceClient

	d *distributor.Distributor
	i *fakeIngester

	roundTripper *tenantRoundTripper
}

func newIngestBenchmark() *ingestBenchmark {
	return &ingestBenchmark{
		clientD: nil,
		d:       nil,
		i:       nil,

		handlerOptions: connectapi.DefaultHandlerOptions(),
		clientOptions:  connectapi.DefaultClientOptions(),
	}
}

func (ib *ingestBenchmark) init(t testing.TB) {
	reg := prometheus.NewRegistry()
	logger := log.NewNopLogger()
	cfg := newConfig(t)
	overrides, err := validation.NewOverrides(cfg.LimitsConfig, &fakeTenantLimits{&cfg.LimitsConfig})
	require.NoError(t, err)

	ring := testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
	}, 1)
	pool := &poolFactory{}

	auth := connect.WithInterceptors(tenant.NewAuthInterceptor(true))

	// create distributor
	ib.d, err = distributor.New(cfg.Distributor, ring, pool, overrides, reg, logger)
	require.NoError(t, err)
	muxD := mux.NewRouter()
	pushv1connect.RegisterPusherServiceHandler(muxD, ib.d, append(ib.handlerOptions, auth)...)
	serverD := httptest.NewServer(muxD)
	t.Cleanup(serverD.Close)
	ib.roundTripper = &tenantRoundTripper{upstream: http.DefaultTransport, tenantID: "tenant-a"}
	httpClient := &http.Client{Transport: ib.roundTripper}
	ib.clientD = pushv1connect.NewPusherServiceClient(httpClient, serverD.URL, ib.clientOptions...)

	// create ingester
	ib.i = &fakeIngester{}
	muxI := mux.NewRouter()
	pushv1connect.RegisterPusherServiceHandler(muxI, ib.i, append(ib.handlerOptions, auth)...)
	serverI := httptest.NewServer(muxI)
	t.Cleanup(serverI.Close)
	clientI := pushv1connect.NewPusherServiceClient(httpClient, serverI.URL, ib.clientOptions...)

	pool.f = func(addr string) (client.PoolClient, error) {
		return &poolClient{clientI}, nil
	}
}

func (ib *ingestBenchmark) push(t testing.TB) {
	_, err := ib.clientD.Push(context.TODO(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Samples: []*pushv1.RawSample{{RawProfile: compressedProfileBytes(t)}},
			Labels: []*typev1.LabelPair{
				{Name: "__name__", Value: "process_cpu"},
				{Name: "foo", Value: "bar"},
				{Name: "region", Value: "global"},
			},
		}},
	}))
	require.NoError(t, err)
}

func TestIngestBenchmark(t *testing.T) {
	ib := newIngestBenchmark()
	ib.init(t)
	ib.push(t)

	require.Equal(t, uint64(1970), ib.i.bytesReceived.Load())
	ib.result(t)

}

func (ib *ingestBenchmark) result(t testing.TB) {
	t.Log("bytes on-wire read", ib.roundTripper.bytesRead.Load())
	t.Log("bytes on-wire sent", ib.roundTripper.bytesSent.Load())
	t.Log("bytes received", ib.i.bytesReceived.Load())
}

func BenchmarkIngestWithoutCompression(b *testing.B) {
	ib := newIngestBenchmark()
	ib.clientOptions = []connect.ClientOption{connectapi.WithoutCompressionClient()}
	ib.handlerOptions = []connect.HandlerOption{connectapi.WithoutCompressionHandler()}
	ib.init(b)

	b.ReportAllocs()
	var expectedBytes uint64
	for n := 0; n < b.N; n++ {
		ib.push(b)
		expectedBytes += 1970
	}
	require.Equal(b, expectedBytes, ib.i.bytesReceived.Load())
	ib.result(b)
}

func BenchmarkIngestWithGzip(b *testing.B) {
	ib := newIngestBenchmark()
	ib.clientOptions = []connect.ClientOption{connectapi.WithGzipClient()}
	ib.handlerOptions = []connect.HandlerOption{connectapi.WithGzipHandler()}
	ib.init(b)

	b.ReportAllocs()
	var expectedBytes uint64
	for n := 0; n < b.N; n++ {
		ib.push(b)
		expectedBytes += 1970
	}
	require.Equal(b, expectedBytes, ib.i.bytesReceived.Load())
	ib.result(b)
}

func BenchmarkIngestWithLZ4(b *testing.B) {
	ib := newIngestBenchmark()
	ib.clientOptions = []connect.ClientOption{connectapi.WithLZ4Client()}
	ib.handlerOptions = []connect.HandlerOption{connectapi.WithLZ4Handler()}
	ib.init(b)

	b.ReportAllocs()
	var expectedBytes uint64
	for n := 0; n < b.N; n++ {
		ib.push(b)
		expectedBytes += 1970
	}
	require.Equal(b, expectedBytes, ib.i.bytesReceived.Load())
	ib.result(b)
}
