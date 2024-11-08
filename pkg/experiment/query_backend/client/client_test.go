package querybackendclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/resolver"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	querybackend "github.com/grafana/pyroscope/pkg/experiment/query_backend"
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
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

func Test_Concurrency(t *testing.T) {
	ports, err := test.GetFreePorts(nServers)
	require.NoError(t, err)

	addresses := make([]string, 0, nServers)
	for i := 0; i < nServers; i++ {
		addresses = append(addresses, fmt.Sprintf("localhost:%d", ports[i]))
	}

	grpcClientCfg := grpcclient.Config{
		MaxRecvMsgSize: 104857600,
		MaxSendMsgSize: 104857600,
	}

	resolver.Register(&multiResolverBuilder{targets: addresses})
	backendAddress := "multi:///"
	cl, err := New(backendAddress, grpcClientCfg)

	backends := make([]*querybackend.QueryBackend, 0, nServers)

	for i := 0; i < nServers; i++ {
		gclInterceptor, err := querybackend.CreateConcurrencyInterceptor(log.NewNopLogger())
		require.NoError(t, err)

		sConfig := createServerConfig(ports[i])
		sConfig.GRPCMiddleware = append(sConfig.GRPCMiddleware, gclInterceptor)

		b, err := querybackend.New(querybackend.Config{
			Address:          backendAddress,
			GRPCClientConfig: grpcClientCfg,
		}, test.NewTestingLogger(t), nil, cl, QueryHandler{})
		require.NoError(t, err)

		serv, err := server.New(sConfig)
		require.NoError(t, err)

		queryv1.RegisterQueryBackendServiceServer(serv.GRPC, b)
		backends = append(backends, b)

		go func() {
			require.NoError(t, serv.Run())
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

func createServerConfig(port int) server.Config {
	sConfig := server.Config{}
	sConfig.GRPCListenAddress = "localhost"
	sConfig.GRPCListenPort = port
	sConfig.GRPCServerMaxSendMsgSize = 4 * 1024 * 1024
	sConfig.GRPCServerMaxRecvMsgSize = 4 * 1024 * 1024
	sConfig.Log = log.NewNopLogger()
	sConfig.Registerer = prometheus.NewRegistry()
	return sConfig
}
