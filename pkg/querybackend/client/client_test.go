package querybackendclient

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/querybackend"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/pkg/test"
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
