package metastoreclient

import (
	"context"
	"fmt"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdiscovery"
	"github.com/hashicorp/raft"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync"
	"testing"
)

const nServers = 3

func TestUnavailable(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := testutil.NewLogger(t)
	c := New(l, grpcclient.Config{}, d)
	ports, err := getFreePorts(nServers)
	assert.NoError(t, err)

	d.On("ServerError", mock.Anything).Run(func(args mock.Arguments) {
	}).Return()

	c.updateServers(createServers(ports))
	res, err := c.AddBlock(context.Background(), &metastorev1.AddBlockRequest{})
	require.Error(t, err)
	require.Nil(t, res)

}

func TestUnavailableRediscover(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := testutil.NewLogger(t)
	config := &grpcclient.Config{}
	flagext.DefaultValues(config)
	c := New(l, *config, d)
	ports, err := getFreePorts(nServers * 2)
	assert.NoError(t, err)

	p1 := ports[:nServers]
	p2 := ports[nServers:]
	m := sync.Mutex{}
	var servers []*mockServer
	defer func() {
		for _, server := range servers {
			server.srv.Stop()
		}
	}()
	callNo := 0
	leaderIndex := -1
	leaderCalled := 0
	_ = leaderIndex
	d.On("ServerError", mock.Anything).Run(func(args mock.Arguments) {
		m.Lock()
		defer m.Unlock()
		if len(servers) == 0 {
			srvInfo := createServers(p2)
			servers = createMockServers(t, p2)
			for i, server := range servers {
				srvIndex := i
				server.addBlock = func(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
					m.Lock()
					defer m.Unlock()
					callNo++
					l.Log("called", srvIndex, "leader", leaderIndex)
					if callNo == 1 {
						leaderIndex = (srvIndex + 1) % nServers
						s, err := status.New(codes.Unavailable, fmt.Sprintf("test error not leader, leader is %s", testServerId(leaderIndex))).
							WithDetails(&typesv1.RaftDetails{
								Leader: string(testServerId(leaderIndex)),
							})
						assert.NoError(t, err)
						return nil, s.Err()
					}
					if callNo == 2 {
						if srvIndex != leaderIndex {
							t.Errorf("expected leader %d to be called, but %d called", leaderIndex, srvIndex)
						}
						leaderCalled++
						return &metastorev1.AddBlockResponse{}, nil
					}
					return nil, fmt.Errorf("unexpected call")
				}
			}
			c.updateServers(srvInfo)
		}
	}).Return()

	c.updateServers(createServers(p1))
	res, err := c.AddBlock(context.Background(), &metastorev1.AddBlockRequest{})
	require.NoError(t, err)
	require.NotNil(t, res)
	m.Lock()
	assert.Equal(t, 2, callNo)
	assert.Equal(t, 1, leaderCalled)
	m.Unlock()

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

func createMockServers(t *testing.T, ports []int) []*mockServer {

	var servers []*mockServer
	for _, port := range ports {
		s := newMockServer(t)
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
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
	return servers

}

func newMockServer(t *testing.T) *mockServer {
	res := &mockServer{
		srv: grpc.NewServer(),
	}
	metastorev1.RegisterMetastoreServiceServer(res.srv, res)
	compactorv1.RegisterCompactionPlannerServer(res.srv, res)
	return res
}

func getFreePorts(len int) (ports []int, err error) {
	ports = make([]int, len)
	for i := 0; i < len; i++ {
		var a *net.TCPAddr
		if a, err = net.ResolveTCPAddr("tcp", "127.0.0.1:0"); err == nil {
			var l *net.TCPListener
			if l, err = net.ListenTCP("tcp", a); err != nil {
				return nil, err
			}
			defer l.Close()
			ports[i] = l.Addr().(*net.TCPAddr).Port
		}
	}
	return ports, nil
}
