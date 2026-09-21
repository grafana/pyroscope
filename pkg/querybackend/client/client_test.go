package querybackendclient

import (
	"context"
	"flag"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/querybackend"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

const (
	nServers            = 12
	nServerResponseTime = 200 * time.Millisecond

	nBlocksInQuery     = 4000
	nConcurrentQueries = 5
)

type QueryHandler struct {
}

func (q QueryHandler) Invoke(ctx context.Context, request *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	time.Sleep(nServerResponseTime)
	return &queryv1.InvokeResponse{}, nil
}

type testQueryBackendServer struct {
	queryv1.UnimplementedQueryBackendServiceServer
	invoke func(context.Context, int32) (*queryv1.InvokeResponse, error)
	calls  atomic.Int32
}

func (s *testQueryBackendServer) Invoke(ctx context.Context, _ *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return s.invoke(ctx, s.calls.Add(1))
}

type multiResolverBuilder struct {
	targets []string
}

func (b *multiResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &multiResolver{
		cc:      cc,
		address: b.targets,
	}
	r.updateState()
	return r, nil
}

func (b *multiResolverBuilder) Scheme() string {
	return "multi"
}

// Resolves all DNS queries to a given set of IPs
//
// Ignores the name being resolved.
type multiResolver struct {
	cc      resolver.ClientConn
	address []string
}

func (r *multiResolver) updateState() {
	addresses := make([]resolver.Address, len(r.address))
	for i, addr := range r.address {
		addresses[i] = resolver.Address{Addr: addr}
	}
	_ = r.cc.UpdateState(resolver.State{Addresses: addresses})
}

func (r *multiResolver) ResolveNow(resolver.ResolveNowOptions) {}

func (r *multiResolver) Close() {}

// oversizedReader answers the first query with a response too large to send and
// any retry with an empty one, so a retry would look like a success.
func oversizedReader(size int) *testQueryBackendServer {
	return &testQueryBackendServer{
		invoke: func(ctx context.Context, call int32) (*queryv1.InvokeResponse, error) {
			if call > 1 {
				return &queryv1.InvokeResponse{}, nil
			}
			return &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_PPROF,
					Pprof: &queryv1.PprofReport{
						Pprof: make([]byte, size),
					},
				}},
			}, nil
		},
	}
}

func TestClient_DoesNotRetryOversizedResponse(t *testing.T) {
	const maxSendMessageSize = 1024

	reader := oversizedReader(2 * maxSendMessageSize)
	backend, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, nil, reader)
	require.NoError(t, err)
	client := newTestClient(t, backend, grpc.MaxSendMsgSize(maxSendMessageSize))

	_, err = client.Invoke(context.Background(), &queryv1.InvokeRequest{
		QueryPlan: &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{Type: queryv1.QueryNode_READ},
		},
	})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.ErrorContains(t, err, "trying to send message larger than max")
	require.Equal(t, int32(1), reader.calls.Load())
}

func TestClient_DoesNotRetryOversizedChildResponse(t *testing.T) {
	const maxSendMessageSize = 1024

	reader := oversizedReader(2 * maxSendMessageSize)
	leafBackend, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, nil, reader)
	require.NoError(t, err)
	leafClient := newTestClient(t, leafBackend, grpc.MaxSendMsgSize(maxSendMessageSize))
	rootBackend, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, leafClient, nil)
	require.NoError(t, err)
	client := newTestClient(t, rootBackend)

	_, err = client.Invoke(context.Background(), &queryv1.InvokeRequest{
		QueryPlan: &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_MERGE,
				Children: []*queryv1.QueryNode{{
					Type: queryv1.QueryNode_READ,
				}},
			},
		},
	})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.ErrorContains(t, err, "trying to send message larger than max")
	require.Equal(t, int32(1), reader.calls.Load())
}

// The concurrency limiter rejects with the status an undeliverable response
// also carries, and no pushback.
func TestClient_RetriesLoadShedding(t *testing.T) {
	reader := &testQueryBackendServer{
		invoke: func(ctx context.Context, call int32) (*queryv1.InvokeResponse, error) {
			if call == 1 {
				return nil, status.Error(codes.ResourceExhausted, "concurrency limit exceeded")
			}
			return &queryv1.InvokeResponse{}, nil
		},
	}
	backend, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, nil, reader)
	require.NoError(t, err)
	client := newTestClient(t, backend)

	resp, err := client.Invoke(context.Background(), &queryv1.InvokeRequest{
		QueryPlan: &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{Type: queryv1.QueryNode_READ},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(2), reader.calls.Load())
}

func TestClient_RetriesRetryableStatus(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.ResourceExhausted} {
		t.Run(code.String(), func(t *testing.T) {
			reader := &testQueryBackendServer{
				invoke: func(ctx context.Context, call int32) (*queryv1.InvokeResponse, error) {
					if call == 1 {
						if err := grpc.SetTrailer(ctx, metadata.Pairs("grpc-retry-pushback-ms", "0")); err != nil {
							return nil, status.Error(codes.Internal, err.Error())
						}
						return nil, status.Error(code, "try again")
					}
					return &queryv1.InvokeResponse{}, nil
				},
			}
			backend, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, nil, reader)
			require.NoError(t, err)
			client := newTestClient(t, backend)

			resp, err := client.Invoke(context.Background(), &queryv1.InvokeRequest{
				QueryPlan: &queryv1.QueryPlan{
					Root: &queryv1.QueryNode{Type: queryv1.QueryNode_READ},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, int32(2), reader.calls.Load())
		})
	}
}

func newTestClient(t *testing.T, backend queryv1.QueryBackendServiceServer, serverOptions ...grpc.ServerOption) *Client {
	t.Helper()

	const address = "localhost:10003"
	listeners, dialOption := test.CreateInMemoryListeners([]string{address})
	server := grpc.NewServer(serverOptions...)
	queryv1.RegisterQueryBackendServiceServer(server, backend)
	go func() {
		_ = server.Serve(listeners[address])
	}()
	t.Cleanup(server.Stop)

	grpcClientConfig := grpcclient.Config{}
	grpcClientConfig.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))
	conn, err := dial("passthrough:///"+address, grpcClientConfig, 5*time.Second, dialOption)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})
	return &Client{grpcClient: queryv1.NewQueryBackendServiceClient(conn)}
}

// Test_Concurrency tests the concurrent invocation of queries against multiple backend servers.
//
// This test sets up a simulated environment with `nServers` gRPC servers, each acting as a
// query backend. It uses `bufconn.Listener` for in-memory gRPC communication to avoid
// actual network I/O.
func Test_Concurrency(t *testing.T) {
	addresses := make([]string, 0, nServers)
	for i := 0; i < nServers; i++ {
		address := fmt.Sprintf("localhost:%d", 10004+i)
		addresses = append(addresses, address)
	}

	listeners, dialOpt := test.CreateInMemoryListeners(addresses)

	grpcClientCfg := grpcclient.Config{}
	grpcClientCfg.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))

	resolver.Register(&multiResolverBuilder{targets: addresses})
	backendAddress := "multi:///"

	cl, err := New(backendAddress, grpcClientCfg, 30*time.Second, dialOpt)
	require.NoError(t, err)

	for i := 0; i < nServers; i++ {
		gclInterceptor, err := querybackend.CreateConcurrencyInterceptor(log.NewNopLogger())
		require.NoError(t, err)

		b, err := querybackend.New(querybackend.Config{
			Address:          backendAddress,
			GRPCClientConfig: grpcClientCfg,
		}, test.NewTestingLogger(t), nil, cl, QueryHandler{})
		require.NoError(t, err)

		grpcOptions := []grpc.ServerOption{
			grpc.ChainUnaryInterceptor(gclInterceptor),
		}
		serv := grpc.NewServer(grpcOptions...)
		require.NoError(t, err)

		queryv1.RegisterQueryBackendServiceServer(serv, b)

		go func() {
			require.NoError(t, serv.Serve(listeners[addresses[i]]))
		}()
	}

	blocks := make([]*metastorev1.BlockMeta, 0, nBlocksInQuery)
	for i := 0; i < nBlocksInQuery; i++ {
		blocks = append(blocks, &metastorev1.BlockMeta{
			Id: fmt.Sprintf("block-%d", i),
		})
	}

	g, ctx := errgroup.WithContext(context.Background())
	for i := 0; i < nConcurrentQueries; i++ {
		g.Go(func() error {
			resp, err := cl.Invoke(ctx, &queryv1.InvokeRequest{
				QueryPlan: queryplan.Build(blocks, 4, 20),
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			return err
		})
	}
	err = g.Wait()
	require.NoError(t, err)
}
