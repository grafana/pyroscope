package metastoreclient

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockcompactorv1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync"
	"testing"
)

var _ metastorev1.MetastoreServiceServer = (*mockServer)(nil)
var _ compactorv1.CompactionPlannerServer = (*mockServer)(nil)

type mockServer struct {
	metastore *mockmetastorev1.MockMetastoreServiceServer
	compactor *mockcompactorv1.MockCompactionPlannerServer

	metastorev1.UnsafeMetastoreServiceServer
	compactorv1.UnsafeCompactionPlannerServer
	srv     *grpc.Server
	id      raft.ServerID
	index   int
	address string
}

func (m *mockServer) GetProfileStats(ctx context.Context, request *metastorev1.GetProfileStatsRequest) (*typesv1.GetProfileStatsResponse, error) {
	return m.metastore.GetProfileStats(ctx, request)
}

func (m *mockServer) PollCompactionJobs(ctx context.Context, request *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
	return m.compactor.PollCompactionJobs(ctx, request)
}

func (m *mockServer) GetCompactionJobs(ctx context.Context, request *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	return m.compactor.GetCompactionJobs(ctx, request)
}

func (m *mockServer) AddBlock(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	return m.metastore.AddBlock(ctx, request)
}

func (m *mockServer) QueryMetadata(ctx context.Context, request *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
	return m.metastore.QueryMetadata(ctx, request)
}

func (m *mockServer) ReadIndex(ctx context.Context, request *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	return m.metastore.ReadIndex(ctx, request)
}

func createServers(ports []int) []discovery.Server {
	var servers []discovery.Server
	for i := 0; i < nServers; i++ {
		servers = append(servers, discovery.Server{
			Raft: raft.Server{
				ID:      testServerId(i),
				Address: raft.ServerAddress(fmt.Sprintf("server-%d", i)),
			},
			ResolvedAddress: fmt.Sprintf("127.0.0.1:%d", ports[i]),
		})
	}
	return servers
}

func testServerId(i int) raft.ServerID {
	return raft.ServerID(fmt.Sprintf("id-%d", i))
}

var _ metastorev1.MetastoreServiceServer = (*mockServer)(nil)
var _ compactorv1.CompactionPlannerServer = (*mockServer)(nil)

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
					WithDetails(&typesv1.RaftDetails{
						Leader: string(testServerId(s.leaderIndex)),
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
		srv.metastore.On("QueryMetadata", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
			return errOrT(&metastorev1.QueryMetadataResponse{}, errf)
		})
		srv.metastore.On("ReadIndex", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
			return errOrT(&metastorev1.ReadIndexResponse{}, errf)
		})
		srv.compactor.On("PollCompactionJobs", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
			return errOrT(&compactorv1.PollCompactionJobsResponse{}, errf)
		})
		srv.compactor.On("GetCompactionJobs", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
			return errOrT(&compactorv1.GetCompactionResponse{}, errf)
		})
		srv.metastore.On("GetProfileStats", mock.Anything, mock.Anything).Maybe().Return(func(context.Context, *metastorev1.GetProfileStatsRequest) (*typesv1.GetProfileStatsResponse, error) {
			return errOrT(&typesv1.GetProfileStatsResponse{}, errf)
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

func createMockServers(t *testing.T, l log.Logger, ports []int) *mockServers {
	var servers []*mockServer
	for idx, port := range ports {
		s := newMockServer(t)
		s.index = idx
		s.id = testServerId(idx)
		s.address = fmt.Sprintf(":%d", port)
		lis, err := net.Listen("tcp", s.address)
		if err != nil {
			assert.NoError(t, err)
		}
		go func() {
			if err := s.srv.Serve(lis); err != nil {
				assert.NoError(t, err)
			}
		}()
		servers = append(servers, s)
	}
	return &mockServers{
		servers: servers,
		t:       t,
		l:       l,
	}

}

func newMockServer(t *testing.T) *mockServer {
	res := &mockServer{
		srv:       grpc.NewServer(),
		metastore: mockmetastorev1.NewMockMetastoreServiceServer(t),
		compactor: mockcompactorv1.NewMockCompactionPlannerServer(t),
	}
	metastorev1.RegisterMetastoreServiceServer(res.srv, res)
	compactorv1.RegisterCompactionPlannerServer(res.srv, res)
	return res
}
