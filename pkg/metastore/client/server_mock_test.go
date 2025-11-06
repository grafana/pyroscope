package metastoreclient

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockraftnodepb"
)

type mockServer struct {
	metastore *mockmetastorev1.MockIndexServiceServer
	compactor *mockmetastorev1.MockCompactionServiceServer
	metadata  *mockmetastorev1.MockMetadataQueryServiceServer
	tenant    *mockmetastorev1.MockTenantServiceServer
	raftNode  *mockraftnodepb.MockRaftNodeServiceServer

	metastorev1.UnsafeIndexServiceServer
	metastorev1.UnsafeCompactionServiceServer
	metastorev1.UnsafeMetadataQueryServiceServer
	metastorev1.UnsafeTenantServiceServer
	raftnodepb.UnsafeRaftNodeServiceServer

	srv     *grpc.Server
	id      raft.ServerID
	index   int
	address string
}

func (m *mockServer) GetTenants(ctx context.Context, request *metastorev1.GetTenantsRequest) (*metastorev1.GetTenantsResponse, error) {
	return m.tenant.GetTenants(ctx, request)
}

func (m *mockServer) GetTenant(ctx context.Context, request *metastorev1.GetTenantRequest) (*metastorev1.GetTenantResponse, error) {
	return m.tenant.GetTenant(ctx, request)
}

func (m *mockServer) DeleteTenant(ctx context.Context, request *metastorev1.DeleteTenantRequest) (*metastorev1.DeleteTenantResponse, error) {
	return m.tenant.DeleteTenant(ctx, request)
}

func (m *mockServer) PollCompactionJobs(ctx context.Context, request *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	return m.compactor.PollCompactionJobs(ctx, request)
}

func (m *mockServer) AddBlock(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	return m.metastore.AddBlock(ctx, request)
}

func (m *mockServer) GetBlockMetadata(ctx context.Context, request *metastorev1.GetBlockMetadataRequest) (*metastorev1.GetBlockMetadataResponse, error) {
	return m.metastore.GetBlockMetadata(ctx, request)
}

func (m *mockServer) QueryMetadata(ctx context.Context, request *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
	return m.metadata.QueryMetadata(ctx, request)
}

func (m *mockServer) QueryMetadataLabels(ctx context.Context, request *metastorev1.QueryMetadataLabelsRequest) (*metastorev1.QueryMetadataLabelsResponse, error) {
	return m.metadata.QueryMetadataLabels(ctx, request)
}

func (m *mockServer) ReadIndex(ctx context.Context, request *raftnodepb.ReadIndexRequest) (*raftnodepb.ReadIndexResponse, error) {
	return m.raftNode.ReadIndex(ctx, request)
}

func (m *mockServer) NodeInfo(ctx context.Context, request *raftnodepb.NodeInfoRequest) (*raftnodepb.NodeInfoResponse, error) {
	return m.raftNode.NodeInfo(ctx, request)
}

func (m *mockServer) RemoveNode(ctx context.Context, request *raftnodepb.RemoveNodeRequest) (*raftnodepb.RemoveNodeResponse, error) {
	return m.raftNode.RemoveNode(ctx, request)
}

func (m *mockServer) AddNode(ctx context.Context, request *raftnodepb.AddNodeRequest) (*raftnodepb.AddNodeResponse, error) {
	return m.raftNode.AddNode(ctx, request)
}

func (m *mockServer) DemoteLeader(ctx context.Context, request *raftnodepb.DemoteLeaderRequest) (*raftnodepb.DemoteLeaderResponse, error) {
	return m.raftNode.DemoteLeader(ctx, request)
}

func (m *mockServer) PromoteToLeader(ctx context.Context, request *raftnodepb.PromoteToLeaderRequest) (*raftnodepb.PromoteToLeaderResponse, error) {
	return m.raftNode.PromoteToLeader(ctx, request)
}

func createServers(ports []int) []discovery.Server {
	var servers []discovery.Server
	for i, p := range ports {
		servers = append(servers, discovery.Server{
			Raft: raft.Server{
				ID:      testServerId(i),
				Address: raft.ServerAddress(fmt.Sprintf("server-%d", i)),
			},
			ResolvedAddress: fmt.Sprintf("127.0.0.1:%d", p),
		})
	}
	return servers
}

func testServerId(i int) raft.ServerID {
	return raft.ServerID(fmt.Sprintf("id-%d", i))
}

var _ metastorev1.IndexServiceServer = (*mockServer)(nil)
var _ metastorev1.CompactionServiceServer = (*mockServer)(nil)

type mockServers struct {
	t       *testing.T
	l       log.Logger
	servers []*mockServer
}

func (m *mockServers) Close() {
	if m == nil {
		return
	}
	for _, s := range m.servers {
		s.srv.Stop()
	}
}

func (m *mockServers) InitWrongLeader() func() {
	type wrongLeaderState struct {
		m            sync.Mutex
		callNo       int
		leaderIndex  int
		leaderCalled int
	}
	s := new(wrongLeaderState)
	s.leaderIndex = -1

	nServers := len(m.servers)
	for _, srv := range m.servers {
		srv := srv
		errf := func() error {
			s.m.Lock()
			defer s.m.Unlock()
			s.callNo++
			m.l.Log("called", srv.index, "leader", s.leaderIndex, "callno", s.callNo)
			if s.callNo == 1 {
				s.leaderIndex = (srv.index + 1) % nServers
				s, err := status.New(codes.Unavailable, fmt.Sprintf("test error not leader, leader is %s", testServerId(s.leaderIndex))).
					WithDetails(&raftnodepb.RaftNode{
						Id: string(testServerId(s.leaderIndex)),
					})
				assert.NoError(m.t, err)
				return s.Err()
			}
			if s.callNo == 2 {
				if srv.index != s.leaderIndex {
					m.t.Errorf("expected leader %d to be called, but %d called", s.leaderIndex, srv.index)
				}
				s.leaderCalled++
				return nil
			}
			m.t.Errorf("unexpected call")
			return fmt.Errorf("unexpected call")
		}
		srv.metastore.On("AddBlock", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
			return errOrT(&metastorev1.AddBlockResponse{}, errf)
		})
		srv.metadata.On("QueryMetadata", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
			return errOrT(&metastorev1.QueryMetadataResponse{}, errf)
		})
		srv.raftNode.On("ReadIndex", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *raftnodepb.ReadIndexRequest) (*raftnodepb.ReadIndexResponse, error) {
			return errOrT(&raftnodepb.ReadIndexResponse{}, errf)
		})
		srv.compactor.On("PollCompactionJobs", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
			return errOrT(&metastorev1.PollCompactionJobsResponse{}, errf)
		})
		srv.tenant.On("GetTenant", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.GetTenantRequest) (*metastorev1.GetTenantResponse, error) {
			return errOrT(&metastorev1.GetTenantResponse{}, errf)
		})
	}
	return func() {
		s.m.Lock()
		assert.Equal(m.t, 2, s.callNo)
		assert.Equal(m.t, 1, s.leaderCalled)
		s.m.Unlock()
	}
}

func errOrT[T any](t *T, f func() error) (*T, error) {
	if err := f(); err != nil {
		return nil, err
	}
	return t, nil
}

// Returns the grpc.DialOptions needed for a client connection to the created mock servers.
func createMockServers(t *testing.T, l log.Logger, dServers []discovery.Server) (*mockServers, []grpc.DialOption) {
	var servers []*mockServer
	listeners, dialOpt := createInMemoryListeners(dServers)
	for idx, dserv := range dServers {
		s := newMockServer(t)
		s.index = idx
		s.id = testServerId(idx)
		s.address = dserv.ResolvedAddress
		go func() {
			if err := s.srv.Serve(listeners[s.address]); err != nil {
				assert.NoError(t, err)
			}
		}()
		servers = append(servers, s)
	}

	ms := &mockServers{
		servers: servers,
		t:       t,
		l:       l,
	}
	return ms, []grpc.DialOption{dialOpt}
}

func createInMemoryListeners(dServers []discovery.Server) (map[string]*bufconn.Listener, grpc.DialOption) {
	addrs := make([]string, 0, len(dServers))
	for _, ds := range dServers {
		addrs = append(addrs, ds.ResolvedAddress)
	}
	return test.CreateInMemoryListeners(addrs)
}

func newMockServer(t *testing.T) *mockServer {
	res := &mockServer{
		srv:       grpc.NewServer(),
		metastore: mockmetastorev1.NewMockIndexServiceServer(t),
		compactor: mockmetastorev1.NewMockCompactionServiceServer(t),
		metadata:  mockmetastorev1.NewMockMetadataQueryServiceServer(t),
		tenant:    mockmetastorev1.NewMockTenantServiceServer(t),
		raftNode:  mockraftnodepb.NewMockRaftNodeServiceServer(t),
	}
	metastorev1.RegisterIndexServiceServer(res.srv, res)
	metastorev1.RegisterCompactionServiceServer(res.srv, res)
	metastorev1.RegisterMetadataQueryServiceServer(res.srv, res)
	metastorev1.RegisterTenantServiceServer(res.srv, res)
	raftnodepb.RegisterRaftNodeServiceServer(res.srv, res)
	return res
}
