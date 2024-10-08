package metastoreclient

import (
	"context"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdiscovery"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

const nServers = 3

func TestUnavailable(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := testutil.NewLogger(t)
	c := New(l, grpcclient.Config{}, d)
	ports, err := test.GetFreePorts(nServers)
	assert.NoError(t, err)

	d.On("Rediscover").Run(func(args mock.Arguments) {
	}).Return()

	c.updateServers(createServers(ports))
	res, err := c.AddBlock(context.Background(), &metastorev1.AddBlockRequest{})
	require.Error(t, err)
	require.Nil(t, res)

}

func TestUnavailable_Rediscover_Wrong_Leader(t *testing.T) {
	t.Run("AddBlock", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.AddBlock(context.Background(), &metastorev1.AddBlockRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
	t.Run("QueryMetadata", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.QueryMetadata(context.Background(), &metastorev1.QueryMetadataRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
	t.Run("ReadIndex", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.ReadIndex(context.Background(), &metastorev1.ReadIndexRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
	t.Run("PollCompactionJobs", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.PollCompactionJobs(context.Background(), &compactorv1.PollCompactionJobsRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
	t.Run("GetCompactionJobs", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.GetCompactionJobs(context.Background(), &compactorv1.GetCompactionRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
	t.Run("GetProfileStats", func(t *testing.T) {
		testRediscoverWrongLeader(t, func(c *Client) {
			res, err := c.GetProfileStats(context.Background(), &metastorev1.GetProfileStatsRequest{})
			require.NoError(t, err)
			require.NotNil(t, res)
		})
	})
}

func testRediscoverWrongLeader(t *testing.T, f func(c *Client)) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := testutil.NewLogger(t)
	config := &grpcclient.Config{}
	flagext.DefaultValues(config)
	c := New(l, *config, d)
	ports, err := test.GetFreePorts(nServers * 2)
	assert.NoError(t, err)

	p1 := ports[:nServers]
	p2 := ports[nServers:]
	m := sync.Mutex{}
	var servers *mockServers
	defer servers.Close()

	verify := func() {}
	d.On("Rediscover", mock.Anything).Run(func(args mock.Arguments) {
		m.Lock()
		defer m.Unlock()
		if servers == nil {
			srvInfo := createServers(p2)
			servers = createMockServers(t, l, p2)
			verify = servers.InitWrongLeader()

			// call updateServers twice
			c.updateServers(srvInfo)
			c.updateServers(srvInfo)
		}
	}).Return()

	c.updateServers(createServers(p1))
	f(c)
	verify()
}

func TestServerError(t *testing.T) {
	d := mockdiscovery.NewMockDiscovery(t)
	d.On("Subscribe", mock.Anything).Return()
	l := testutil.NewLogger(t)
	c := New(l, grpcclient.Config{}, d)

	d.On("Rediscover").Run(func(args mock.Arguments) {
	}).Return()

	res, err := c.AddBlock(context.Background(), &metastorev1.AddBlockRequest{})
	require.Error(t, err)
	require.Nil(t, res)
}
