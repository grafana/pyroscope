package metastore

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/services"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdiscovery"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"testing"
	"time"
)

func TestRaftDetailsAddBlock(t *testing.T) {
	cfg := new(Config)
	flagext.DefaultValues(cfg)

	ms := createMetastore(t, cfg, 3)
	defer ms.Close()

	errors := 0
	for _, it := range ms.instances {
		_, err := it.metastoreClient.AddBlock(context.Background(), &metastorev1.AddBlockRequest{
			Block: &metastorev1.BlockMeta{},
		})
		if err != nil {

			requireRaftDetails(t, err)
			errors++
		}
	}
	require.Equal(t, 2, errors)
}

func TestRaftDetailsPullCompaction(t *testing.T) {
	cfg := new(Config)
	flagext.DefaultValues(cfg)

	ms := createMetastore(t, cfg, 3)
	defer ms.Close()

	errors := 0
	for _, it := range ms.instances {
		_, err := it.compactorClient.PollCompactionJobs(context.Background(), &compactorv1.PollCompactionJobsRequest{})
		if err != nil {
			requireRaftDetails(t, err)
			errors++
		}
	}
	require.Equal(t, 2, errors)
}

func requireRaftDetails(t *testing.T, err error) {
	t.Log("error", err)
	s, ok := status.FromError(err)
	detailsLeader := ""
	if ok && s.Code() == codes.Unavailable {
		ds := s.Details()
		if len(ds) > 0 {
			for _, d := range ds {
				if rd, ok := d.(*typesv1.RaftDetails); ok {
					detailsLeader = rd.Leader
					break
				}
			}
		}
	}
	t.Log("leader is", detailsLeader)
	require.NotEmpty(t, detailsLeader)
}

func createMetastore(t *testing.T, cfg *Config, n int) metastoreSet {
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
	configs := make([]Config, n)
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

	d := mockStaticDiscovery(t, servers)
	cl := metastoreclient.New(l, cfg.GRPCClientConfig, d)
	err = cl.Service().StartAsync(context.Background())
	require.NoError(t, err)

	l.Log("grpcAddresses", fmt.Sprintf("%+v", grpcAddresses), "raftAddresses", fmt.Sprintf("%+v", raftAddresses))
	res := metastoreSet{
		t: t,
	}
	for i := 0; i < n; i++ {
		options, err := cfg.GRPCClientConfig.DialOption(nil, nil)
		require.NoError(t, err)
		cc, err := grpc.Dial(grpcAddresses[i], options...)
		require.NoError(t, err)
		il := log.With(l, "idx", bootstrapPeers[i])
		server := grpc.NewServer()
		m, err := New(configs[i], nil, il, prometheus.NewRegistry(), cl)
		require.NoError(t, err)
		metastorev1.RegisterMetastoreServiceServer(server, m)
		compactorv1.RegisterCompactionPlannerServer(server, m)
		lis, err := net.Listen("tcp", grpcAddresses[i])
		assert.NoError(t, err)
		go func() {
			err := server.Serve(lis)
			assert.NoError(t, err)
		}()
		res.instances = append(res.instances, metastoreInstance{
			metastore:       m,
			srv:             servers[i],
			cc:              cc,
			metastoreClient: metastorev1.NewMetastoreServiceClient(cc),
			compactorClient: compactorv1.NewCompactionPlannerClient(cc),
			server:          server,
		})
		err = m.Service().StartAsync(context.Background())
		il.Log("msg", "service started")
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		for i := 0; i < n; i++ {
			if res.instances[i].metastore.Service().State() != services.Running {
				return false
			}
			if res.instances[i].metastore.CheckReady(context.Background()) != nil {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond)

	res.client = cl

	return res
}

func mockStaticDiscovery(t *testing.T, servers []discovery.Server) *mockdiscovery.MockDiscovery {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Run(func(args mock.Arguments) {
		upd := args.Get(0).(discovery.Updates)
		upd.Servers(servers)
	})
	d.On("ServerError", mock.Anything).Return()
	d.On("Close").Return(nil)
	return d
}

type metastoreInstance struct {
	metastore       *Metastore
	srv             discovery.Server
	server          *grpc.Server
	cc              *grpc.ClientConn
	metastoreClient metastorev1.MetastoreServiceClient
	compactorClient compactorv1.CompactionPlannerClient
}

func (i *metastoreInstance) client() metastorev1.MetastoreServiceClient {
	return i.metastoreClient
}

type metastoreSet struct {
	t         *testing.T
	instances []metastoreInstance
	client    *metastoreclient.Client
}

func (m *metastoreSet) Close() {
	for _, i := range m.instances {
		i.metastore.Service().StopAsync()
		err := i.metastore.Service().AwaitTerminated(context.Background())
		require.NoError(m.t, err)
		i.cc.Close()
		i.server.Stop()
	}
	m.client.Service().StopAsync()
	err := m.client.Service().AwaitTerminated(context.Background())
	require.NoError(m.t, err)
}
