package metastoreclient

import (
	"context"
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockdiscovery"
)

func TestNodeInfoAll(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := test.NewTestingLogger(t)
	config := &grpcclient.Config{}
	flagext.DefaultValues(config)

	dServers := createServers([]int{50031, 50032, 50033})
	servers, dialOpts := createMockServers(t, l, dServers)
	defer servers.Close()
	for i, s := range servers.servers {
		if i == 2 {
			s.raftNode.On("NodeInfo", mock.Anything, mock.Anything).
				Return(nil, status.Error(codes.Unavailable, "unavailable"))
			continue
		}
		s.raftNode.On("NodeInfo", mock.Anything, mock.Anything).
			Return(&raftnodepb.NodeInfoResponse{Node: &raftnodepb.NodeInfo{
				ServerId:            string(s.id),
				SupportedFsmVersion: uint32(i + 1),
			}}, nil)
	}

	c := New(l, *config, d, dialOpts...)
	c.updateServers(dServers)

	infos, err := c.NodeInfoAll(context.Background())
	require.ErrorContains(t, err, string(testServerId(2)))

	got := make(map[string]uint32)
	for _, info := range infos {
		got[info.ServerId] = info.SupportedFsmVersion
	}
	require.Equal(t, map[string]uint32{
		string(testServerId(0)): 1,
		string(testServerId(1)): 2,
	}, got)
}

func TestNodeInfoAll_NoServers(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	c := New(test.NewTestingLogger(t), grpcclient.Config{}, d)
	infos, err := c.NodeInfoAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, infos)
}
