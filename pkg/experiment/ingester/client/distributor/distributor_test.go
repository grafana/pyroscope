package distributor

import (
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor/placement"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

// TODO(kolesnikovae): Test distribution fairness.

var (
	testLabels = []*typesv1.LabelPair{
		{Name: "foo", Value: "bar"},
		{Name: "baz", Value: "qux"},
		{Name: "service_name", Value: "my-service"},
	}
	testInstances = []ring.InstanceDesc{
		{Addr: "a", Tokens: make([]uint32, 4)},
		{Addr: "b", Tokens: make([]uint32, 4)},
		{Addr: "c", State: ring.LEAVING, Tokens: make([]uint32, 4)},
	}
)

type mockDistributionStrategy struct {
	mock.Mock
}

func (m *mockDistributionStrategy) NumTenantShards(k placement.Key, n uint32) (size uint32) {
	return m.Called(k, n).Get(0).(uint32)
}

func (m *mockDistributionStrategy) NumDatasetShards(k placement.Key, n uint32) (size uint32) {
	return m.Called(k, n).Get(0).(uint32)
}

func (m *mockDistributionStrategy) PickShard(k placement.Key, n uint32) (shard uint32) {
	return m.Called(k, n).Get(0).(uint32)
}

func Test_EmptyRing(t *testing.T) {
	m := new(mockDistributionStrategy)
	d := NewDistributor(m)
	r := testhelper.NewMockRing(nil, 1)

	k := NewTenantServiceDatasetKey("", nil)
	_, err := d.Distribute(k, r)
	assert.ErrorIs(t, err, ring.ErrEmptyRing)
}

func Test_Distribution_AvailableShards(t *testing.T) {
	for _, tc := range []struct {
		description   string
		tenantShards  uint32
		datasetShards uint32
	}{
		{description: "zero", tenantShards: 0, datasetShards: 0},
		{description: "min", tenantShards: 1, datasetShards: 1},
		{description: "insufficient", tenantShards: 1 << 10, datasetShards: 1 << 9},
		{description: "invalid", tenantShards: 1 << 10, datasetShards: 2 << 10},
	} {
		t.Run(tc.description, func(t *testing.T) {
			k := NewTenantServiceDatasetKey("tenant-a", testLabels)
			m := new(mockDistributionStrategy)
			m.On("NumTenantShards", k, mock.Anything).Return(tc.tenantShards).Once()
			m.On("NumDatasetShards", k, mock.Anything).Return(tc.datasetShards).Once()
			m.On("PickShard", k, mock.Anything).Return(uint32(0)).Once()

			d := NewDistributor(m)
			r := testhelper.NewMockRing(testInstances, 1)
			p, err := d.Distribute(k, r)
			require.NoError(t, err)
			c := make([]*ring.InstanceDesc, 0, 2)
			for {
				x, ok := p.Next()
				if !ok {
					break
				}
				c = append(c, x)
			}

			assert.Equal(t, 2, len(c))
			m.AssertExpectations(t)
		})
	}
}

func Test_RingUpdate(t *testing.T) {
	k := NewTenantServiceDatasetKey("", nil)
	m := new(mockDistributionStrategy)
	m.On("NumTenantShards", k, mock.Anything).Return(uint32(1))
	m.On("NumDatasetShards", k, mock.Anything).Return(uint32(1))
	m.On("PickShard", k, mock.Anything).Return(uint32(0))

	d := NewDistributor(m)
	r := testhelper.NewMockRing(testInstances, 1)
	_, err := d.Distribute(k, r)
	require.NoError(t, err)

	instances := make([]ring.InstanceDesc, 2)
	copy(instances, testInstances[:1])
	r.SetInstances(instances)
	require.NoError(t, d.Update(r))

	p, err := d.Distribute(k, r)
	require.NoError(t, err)
	c := make([]*ring.InstanceDesc, 0, 1)
	for {
		x, ok := p.Next()
		if !ok {
			break
		}
		c = append(c, x)
	}

	// Only one instance is available.
	assert.Equal(t, 1, len(c))
	m.AssertExpectations(t)
}
