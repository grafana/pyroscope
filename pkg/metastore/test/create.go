package test

import (
	"context"
	"fmt"
	"net"
	"sync"
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
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/test/bufconn"

	"github.com/grafana/pyroscope/v2/pkg/metastore"
	metastoreclient "github.com/grafana/pyroscope/v2/pkg/metastore/client"
	"github.com/grafana/pyroscope/v2/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	placement "github.com/grafana/pyroscope/v2/pkg/segmentwriter/client/distributor/placement/adaptiveplacement"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockdiscovery"
	"github.com/grafana/pyroscope/v2/pkg/util/health"
	"github.com/grafana/pyroscope/v2/pkg/validation"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type InstanceOption func(i int, cfg *metastore.Config)

func NewMetastoreSet(t *testing.T, cfg *metastore.Config, n int, bucket objstore.Bucket, opts ...InstanceOption) MetastoreSet {
	l := test.NewTestingLogger(t)

	grpcAddresses := make([]string, n)
	raftAddresses := make([]string, n)
	raftIds := make([]string, n)
	bootstrapPeers := make([]string, n)
	raftPorts, err := test.GetFreePorts(n)
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		grpcAddresses[i] = fmt.Sprintf("localhost:%d", 10500+i)
		raftAddresses[i] = fmt.Sprintf("localhost:%d", raftPorts[i])
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
		icfg.Raft.SnapshotsDir = icfg.Raft.Dir
		icfg.Raft.AdvertiseAddress = raftAddresses[i]
		icfg.Raft.BindAddress = raftAddresses[i]
		icfg.Raft.BootstrapPeers = bootstrapPeers
		icfg.Raft.BootstrapExpectPeers = n
		for _, opt := range opts {
			opt(i, &icfg)
		}
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

	listeners, dialOpt := newInMemoryListeners(grpcAddresses)
	d := MockStaticDiscovery(t, servers)
	client := metastoreclient.New(l, cfg.GRPCClientConfig, d, dialOpt, fastReconnect)
	err = client.Service().StartAsync(context.Background())
	require.NoError(t, err)

	res := MetastoreSet{
		t:         t,
		logger:    l,
		bucket:    bucket,
		configs:   configs,
		listeners: listeners,
		dialOpt:   dialOpt,
		Instances: make([]MetastoreInstance, n),
		running:   make([]bool, n),
		Client:    client,
	}

	for i := 0; i < n; i++ {
		require.NoError(t, res.StartInstance(i))
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

	return res
}

var fastReconnect = grpc.WithConnectParams(grpc.ConnectParams{
	Backoff: backoff.Config{
		BaseDelay:  50 * time.Millisecond,
		Multiplier: 1.6,
		MaxDelay:   500 * time.Millisecond,
	},
	MinConnectTimeout: time.Second,
})

type inMemoryListeners struct {
	mu        sync.Mutex
	listeners map[string]*bufconn.Listener
}

func newInMemoryListeners(addresses []string) (*inMemoryListeners, grpc.DialOption) {
	l := &inMemoryListeners{listeners: make(map[string]*bufconn.Listener)}
	for _, a := range addresses {
		l.reset(a)
	}
	dialer := func(_ context.Context, address string) (net.Conn, error) {
		if el := l.get(address); el != nil {
			return el.Dial()
		}
		return net.Dial("tcp", address)
	}
	return l, grpc.WithContextDialer(dialer)
}

func (l *inMemoryListeners) get(address string) *bufconn.Listener {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.listeners[address]
}

func (l *inMemoryListeners) reset(address string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listeners[address] = bufconn.Listen(256 << 10)
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
	Config     metastore.Config
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

	logger    log.Logger
	bucket    objstore.Bucket
	configs   []metastore.Config
	listeners *inMemoryListeners
	dialOpt   grpc.DialOption
	running   []bool
}

func (m *MetastoreSet) StartInstance(i int) error {
	cfg := m.configs[i]
	options, err := cfg.GRPCClientConfig.DialOption(nil, nil, nil)
	if err != nil {
		return err
	}
	options = append(options, m.dialOpt, fastReconnect)
	cc, err := grpc.Dial(cfg.Address, options...)
	if err != nil {
		return err
	}
	logger := log.With(m.logger, "idx", cfg.Raft.ServerID)
	registry := prometheus.NewRegistry()
	placementManager := placement.NewManager(
		logger,
		registry,
		placement.DefaultConfig(),
		validation.MockDefaultOverrides(),
		placement.NewStore(m.bucket),
	)
	ms, err := metastore.New(cfg, validation.MockDefaultOverrides(), logger, registry, health.NoOpService, m.Client, m.bucket, placementManager)
	if err != nil {
		_ = cc.Close()
		return err
	}
	server := grpc.NewServer()
	ms.Register(server)
	lis := m.listeners.get(cfg.Address)
	go func() {
		assert.NoError(m.t, server.Serve(lis))
	}()
	m.Instances[i] = MetastoreInstance{
		Config:     cfg,
		Metastore:  ms,
		Connection: cc,
		Server:     server,

		IndexServiceClient:         metastorev1.NewIndexServiceClient(cc),
		CompactionServiceClient:    metastorev1.NewCompactionServiceClient(cc),
		MetadataQueryServiceClient: metastorev1.NewMetadataQueryServiceClient(cc),
		TenantServiceClient:        metastorev1.NewTenantServiceClient(cc),
		RaftNodeServiceClient:      raftnodepb.NewRaftNodeServiceClient(cc),
	}
	ctx := context.Background()
	if err = ms.Service().StartAsync(ctx); err != nil {
		return err
	}
	if err = ms.Service().AwaitRunning(ctx); err != nil {
		return err
	}
	m.running[i] = true
	logger.Log("msg", "service started")
	return nil
}

func (m *MetastoreSet) StopInstance(i int) {
	if !m.running[i] {
		return
	}
	it := m.Instances[i]
	it.Metastore.Service().StopAsync()
	require.NoError(m.t, it.Metastore.Service().AwaitTerminated(context.Background()))
	_ = it.Connection.Close()
	it.Server.Stop()
	m.listeners.reset(m.configs[i].Address)
	m.running[i] = false
}

func (m *MetastoreSet) Configure(i int, configure func(*metastore.Config)) {
	configure(&m.configs[i])
}

func (m *MetastoreSet) RestartInstance(i int, configure func(*metastore.Config)) {
	m.StopInstance(i)
	if configure != nil {
		m.Configure(i, configure)
	}
	require.NoError(m.t, m.StartInstance(i))
}

func (m *MetastoreSet) WipeInstance(i int) {
	require.False(m.t, m.running[i], "instance must be stopped before wiping its data")
	m.configs[i].FSM.DataDir = m.t.TempDir()
	m.configs[i].Raft.Dir = m.t.TempDir()
	m.configs[i].Raft.SnapshotsDir = m.configs[i].Raft.Dir
}

func (m *MetastoreSet) Close() {
	for i := range m.Instances {
		m.StopInstance(i)
	}
	m.Client.Service().StopAsync()
	err := m.Client.Service().AwaitTerminated(context.Background())
	require.NoError(m.t, err)
}
