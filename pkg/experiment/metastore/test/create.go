package test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdiscovery"
	"github.com/grafana/pyroscope/pkg/validation"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc"
)

func NewMetastoreSet(t *testing.T, cfg *metastore.Config, n int, bucket objstore.Bucket) MetastoreSet {
	l := test.NewTestingLogger(t)

	ports, err := test.GetFreePorts(2 * n)
	addresses := make([]string, 2*n)
	for i, port := range ports {
		addresses[i] = fmt.Sprintf("localhost:%d", port)
	}
	grpcAddresses := addresses[:n]
	raftAddresses := addresses[n:]
	raftIds := make([]string, n)
	for i := 0; i < n; i++ {
		raftIds[i] = fmt.Sprintf("node-%d", i)
	}
	bootstrapPeers := make([]string, n)
	configs := make([]metastore.Config, n)
	servers := make([]discovery.Server, n)

	for i := 0; i < n; i++ {
		bootstrapPeers[i] = fmt.Sprintf("%s/%s", raftAddresses[i], raftIds[i])

		icfg := *cfg
		icfg.MinReadyDuration = 0
		icfg.Address = grpcAddresses[i]
		icfg.DataDir = t.TempDir()
		icfg.Raft.ServerID = raftIds[i]
		icfg.Raft.Dir = t.TempDir()
		icfg.Raft.AdvertiseAddress = raftAddresses[i]
		icfg.Raft.BindAddress = raftAddresses[i]
		icfg.Raft.BootstrapPeers = bootstrapPeers
		icfg.Raft.BootstrapExpectPeers = n
		srv := discovery.Server{
			Raft: raft.Server{
				ID:      raft.ServerID(raftIds[i]),
				Address: raft.ServerAddress(raftAddresses[i]),
			},
			ResolvedAddress: addresses[i],
		}
		servers[i] = srv
		configs[i] = icfg
	}
	require.NoError(t, err)

	d := MockStaticDiscovery(t, servers)
	client := metastoreclient.New(l, cfg.GRPCClientConfig, d)
	err = client.Service().StartAsync(context.Background())
	require.NoError(t, err)

	l.Log("grpcAddresses", fmt.Sprintf("%+v", grpcAddresses), "raftAddresses", fmt.Sprintf("%+v", raftAddresses))
	res := MetastoreSet{
		t: t,
	}
	for i := 0; i < n; i++ {
		options, err := cfg.GRPCClientConfig.DialOption(nil, nil)
		require.NoError(t, err)
		cc, err := grpc.Dial(grpcAddresses[i], options...)
		require.NoError(t, err)
		logger := log.With(l, "idx", bootstrapPeers[i])
		server := grpc.NewServer()
		registry := prometheus.NewRegistry()
		placementManager := adaptive_placement.NewManager(
			logger,
			registry,
			adaptive_placement.DefaultConfig(),
			validation.MockDefaultOverrides(),
			adaptive_placement.NewStore(bucket),
		)
		m, err := metastore.New(configs[i], logger, registry, client, bucket, placementManager)
		require.NoError(t, err)
		metastorev1.RegisterMetastoreServiceServer(server, m)
		compactorv1.RegisterCompactionPlannerServer(server, m)
		lis, err := net.Listen("tcp", grpcAddresses[i])
		assert.NoError(t, err)
		go func() {
			err := server.Serve(lis)
			assert.NoError(t, err)
		}()
		res.Instances = append(res.Instances, MetastoreInstance{
			Metastore:               m,
			Connection:              cc,
			MetastoreInstanceClient: metastorev1.NewMetastoreServiceClient(cc),
			CompactorInstanceClient: compactorv1.NewCompactionPlannerClient(cc),
			Server:                  server,
		})
		err = m.Service().StartAsync(context.Background())
		logger.Log("msg", "service started")
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		for i := 0; i < n; i++ {
			if res.Instances[i].Metastore.Service().State() != services.Running {
				return false
			}
			if res.Instances[i].Metastore.CheckReady(context.Background()) != nil {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond)

	res.Client = client

	return res
}

func MockStaticDiscovery(t *testing.T, servers []discovery.Server) *mockdiscovery.MockDiscovery {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Run(func(args mock.Arguments) {
		upd := args.Get(0).(discovery.Updates)
		upd.Servers(servers)
	})
	d.On("Rediscover", mock.Anything).Return()
	d.On("Close").Return(nil)
	return d
}

type MetastoreInstance struct {
	Metastore               *metastore.Metastore
	Server                  *grpc.Server
	Connection              *grpc.ClientConn
	MetastoreInstanceClient metastorev1.MetastoreServiceClient
	CompactorInstanceClient compactorv1.CompactionPlannerClient
}

func (i *MetastoreInstance) client() metastorev1.MetastoreServiceClient {
	return i.MetastoreInstanceClient
}

type MetastoreSet struct {
	t         *testing.T
	Instances []MetastoreInstance
	Client    *metastoreclient.Client
}

func (m *MetastoreSet) Close() {
	for _, i := range m.Instances {
		i.Metastore.Service().StopAsync()
		err := i.Metastore.Service().AwaitTerminated(context.Background())
		require.NoError(m.t, err)
		i.Connection.Close()
		i.Server.Stop()
	}
	m.Client.Service().StopAsync()
	err := m.Client.Service().AwaitTerminated(context.Background())
	require.NoError(m.t, err)
}
