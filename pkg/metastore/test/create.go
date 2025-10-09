package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
	"github.com/grafana/pyroscope/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	placement "github.com/grafana/pyroscope/pkg/segmentwriter/client/distributor/placement/adaptiveplacement"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdiscovery"
	"github.com/grafana/pyroscope/pkg/util/health"
	"github.com/grafana/pyroscope/pkg/validation"
)

func NewMetastoreSet(t *testing.T, cfg *metastore.Config, n int, bucket objstore.Bucket) MetastoreSet {
	l := test.NewTestingLogger(t)

	grpcAddresses := make([]string, n)
	raftAddresses := make([]string, n)
	raftIds := make([]string, n)
	bootstrapPeers := make([]string, n)
	for i := 0; i < n; i++ {
		grpcAddresses[i] = fmt.Sprintf("localhost:%d", 10500+i)
		raftAddresses[i] = fmt.Sprintf("localhost:%d", 10500+2*i)
		raftIds[i] = fmt.Sprintf("node-%d", i)
		bootstrapPeers[i] = fmt.Sprintf("%s/%s", raftAddresses[i], raftIds[i])
	}
	l.Log("grpcAddresses", fmt.Sprintf("%+v", grpcAddresses), "raftAddresses", fmt.Sprintf("%+v", raftAddresses))

	configs := make([]metastore.Config, n)
	for i := 0; i < n; i++ {
		icfg := *cfg
		icfg.MinReadyDuration = 0
		icfg.Address = grpcAddresses[i]
		icfg.FSM.DataDir = t.TempDir()
		icfg.Raft.ServerID = raftIds[i]
		icfg.Raft.Dir = t.TempDir()
		icfg.Raft.AdvertiseAddress = raftAddresses[i]
		icfg.Raft.BindAddress = raftAddresses[i]
		icfg.Raft.BootstrapPeers = bootstrapPeers
		icfg.Raft.BootstrapExpectPeers = n
		configs[i] = icfg
	}

	servers := make([]discovery.Server, n)
	for i := 0; i < n; i++ {
		srv := discovery.Server{
			Raft: raft.Server{
				ID:      raft.ServerID(raftIds[i]),
				Address: raft.ServerAddress(raftAddresses[i]),
			},
			ResolvedAddress: grpcAddresses[i],
		}
		servers[i] = srv
	}

	listeners, dialOpt := test.CreateInMemoryListeners(grpcAddresses)
	d := MockStaticDiscovery(t, servers)
	client := metastoreclient.New(l, cfg.GRPCClientConfig, d, dialOpt)
	err := client.Service().StartAsync(context.Background())
	require.NoError(t, err)

	res := MetastoreSet{
		t: t,
	}

	for i := 0; i < n; i++ {
		options, err := cfg.GRPCClientConfig.DialOption(nil, nil, nil)
		require.NoError(t, err)
		options = append(options, dialOpt)
		cc, err := grpc.Dial(grpcAddresses[i], options...)
		require.NoError(t, err)
		logger := log.With(l, "idx", bootstrapPeers[i])
		server := grpc.NewServer()
		registry := prometheus.NewRegistry()
		placementManager := placement.NewManager(
			logger,
			registry,
			placement.DefaultConfig(),
			validation.MockDefaultOverrides(),
			placement.NewStore(bucket),
		)
		m, err := metastore.New(configs[i], validation.MockDefaultOverrides(), logger, registry, health.NoOpService, client, bucket, placementManager)
		require.NoError(t, err)
		m.Register(server)

		lis := listeners[grpcAddresses[i]]
		go func() {
			err := server.Serve(lis)
			assert.NoError(t, err)
		}()
		res.Instances = append(res.Instances, MetastoreInstance{
			Metastore:  m,
			Connection: cc,
			Server:     server,

			IndexServiceClient:         metastorev1.NewIndexServiceClient(cc),
			CompactionServiceClient:    metastorev1.NewCompactionServiceClient(cc),
			MetadataQueryServiceClient: metastorev1.NewMetadataQueryServiceClient(cc),
			TenantServiceClient:        metastorev1.NewTenantServiceClient(cc),
			RaftNodeServiceClient:      raftnodepb.NewRaftNodeServiceClient(cc),
		})
		service := m.Service()
		ctx := context.Background()
		require.NoError(t, service.StartAsync(ctx))
		require.NoError(t, service.AwaitRunning(ctx))
		logger.Log("msg", "service started")
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
	Metastore  *metastore.Metastore
	Server     *grpc.Server
	Connection *grpc.ClientConn

	metastorev1.IndexServiceClient
	metastorev1.CompactionServiceClient
	metastorev1.MetadataQueryServiceClient
	metastorev1.TenantServiceClient
	raftnodepb.RaftNodeServiceClient
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
