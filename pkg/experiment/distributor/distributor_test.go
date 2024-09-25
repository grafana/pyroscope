package distributor

import (
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
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
		{Addr: "a", Tokens: make([]uint32, 1)},
		{Addr: "b", Tokens: make([]uint32, 1)},
		{Addr: "c", State: ring.LEAVING, Tokens: make([]uint32, 1)},
	}
)

type mockDistributionStrategy struct{ mock.Mock }

func (m *mockDistributionStrategy) Place(k placement.Key) *placement.Placement {
	return m.Called(k).Get(0).(*placement.Placement)
}

func (m *mockDistributionStrategy) NumTenantShards(k placement.Key, n int) (size int) {
	return m.Called(k, n).Get(0).(int)
}

func (m *mockDistributionStrategy) NumDatasetShards(k placement.Key, n int) (size int) {
	return m.Called(k, n).Get(0).(int)
}

func (m *mockDistributionStrategy) PickShard(k placement.Key, n int) (shard int) {
	return m.Called(k, n).Get(0).(int)
}

func Test_EmptyRing(t *testing.T) {
	m := new(mockDistributionStrategy)
	d := NewDistributor(m)
	r := testhelper.NewMockRing(nil, 1)

	k := NewTenantServiceDatasetKey("")
	m.On("Place", k).Return((*placement.Placement)(nil)).Once()
	_, err := d.Distribute(k, r)
	assert.ErrorIs(t, err, ring.ErrEmptyRing)
}

func Test_Distribution_AvailableShards(t *testing.T) {
	for _, tc := range []struct {
		description   string
		tenantShards  int
		datasetShards int
	}{
		{description: "zero", tenantShards: 0, datasetShards: 0},
		{description: "min", tenantShards: 1, datasetShards: 1},
		{description: "insufficient", tenantShards: 1 << 10, datasetShards: 1 << 9},
		{description: "invalid", tenantShards: 1 << 10, datasetShards: 2 << 10},
	} {
		t.Run(tc.description, func(t *testing.T) {
			k := NewTenantServiceDatasetKey("tenant-a", testLabels...)
			m := new(mockDistributionStrategy)
			m.On("Place", k).Return((*placement.Placement)(nil)).Once()
			m.On("NumTenantShards", k, mock.Anything).Return(tc.tenantShards).Once()
			m.On("NumDatasetShards", k, mock.Anything).Return(tc.datasetShards).Once()
			m.On("PickShard", k, mock.Anything).Return(0).Once()

			d := NewDistributor(m)
			r := testhelper.NewMockRing(testInstances, 1)
			p, err := d.Distribute(k, r)
			require.NoError(t, err)
			c := make([]ring.InstanceDesc, 0, 2)
			for p.Instances.Next() {
				c = append(c, p.Instances.At())
			}

			assert.Equal(t, 3, len(c))
			m.AssertExpectations(t)
		})
	}
}

func Test_RingUpdate(t *testing.T) {
	k := NewTenantServiceDatasetKey("")
	m := new(mockDistributionStrategy)
	m.On("Place", k).Return((*placement.Placement)(nil))
	m.On("NumTenantShards", k, mock.Anything).Return(1)
	m.On("NumDatasetShards", k, mock.Anything).Return(1)
	m.On("PickShard", k, mock.Anything).Return(0)

	d := NewDistributor(m)
	r := testhelper.NewMockRing(testInstances, 1)
	_, err := d.Distribute(k, r)
	require.NoError(t, err)

	instances := make([]ring.InstanceDesc, 2)
	copy(instances, testInstances[:1])
	r.SetInstances(instances)
	require.NoError(t, d.updateDistribution(r, 0))

	p, err := d.Distribute(k, r)
	require.NoError(t, err)
	c := make([]ring.InstanceDesc, 0, 1)
	for p.Instances.Next() {
		c = append(c, p.Instances.At())
	}

	// Only one instance is available.
	assert.Equal(t, 1, len(c))
	m.AssertExpectations(t)
}

func Test_Distribution_FallbackLocations(t *testing.T) {
	m := new(mockDistributionStrategy)
	m.On("Place", mock.Anything).Return((*placement.Placement)(nil))
	m.On("NumTenantShards", mock.Anything, mock.Anything).Return(8)
	m.On("NumDatasetShards", mock.Anything, mock.Anything).Return(4)

	d := NewDistributor(m)
	d.LocationsPerKey = 5

	r := testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "a", Tokens: make([]uint32, 4)},
		{Addr: "b", Tokens: make([]uint32, 4)},
		{Addr: "c", Tokens: make([]uint32, 4)},
	}, 1)

	collect := func(pick int) []string {
		k := NewTenantServiceDatasetKey("tenant-a")
		m.On("PickShard", mock.Anything, mock.Anything).Return(pick).Once()
		p, err := d.Distribute(k, r)
		require.NoError(t, err)
		var instances []string
		for p.Instances.Next() {
			instances = append(instances, p.Instances.At().Addr)
		}
		return instances
	}

	//   0 1 2 3 4 5 6 7 8 9 10 11  all shards
	//   * * * *         > * *  *   tenant (size 8, offset 8)
	//       > *         * *        dataset (size 4, offset 6+8 mod 12 = 2)
	//   a a a b b b c c a b c  c   shuffling (see d.distribution.shards)
	//   ----------------------------------------------------------------------
	//       0 1         2 3 4      PickShard 0 (offset within dataset)
	//                       ^ borrowed from the tenant
	//
	//       3 0         1 2 4      PickShard 1
	//       2 3         0 1 4      PickShard 2
	//       1 2         3 0 4      PickShard 3

	// Identical keys have identical placement.
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(0))
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(0))

	assert.Equal(t, []string{"b", "a", "b", "a", "c"}, collect(1))
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(2))
	assert.Equal(t, []string{"b", "a", "b", "a", "c"}, collect(3))
}

func Test_permutation(t *testing.T) {
	actual := make([][]uint32, 0, 16)
	copyP := func(s []uint32) []uint32 {
		c := make([]uint32, len(s))
		copy(c, s)
		return c
	}

	var p perm
	for i := 0; i <= 32; i += 4 {
		p.resize(i)
		actual = append(actual, copyP(p.v))
	}
	for i := 32; i >= 0; i -= 4 {
		p.resize(i)
		actual = append(actual, copyP(p.v))
	}

	expected := [][]uint32{
		{},
		{2, 3, 1, 0},
		{2, 3, 1, 5, 6, 4, 7, 0},
		{2, 3, 1, 5, 6, 4, 9, 11, 0, 7, 8, 10},
		{2, 3, 1, 5, 12, 4, 14, 11, 15, 7, 8, 13, 6, 10, 9, 0},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 7, 8, 13, 6, 10, 9, 19, 17, 16, 1, 0},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 22, 8, 13, 6, 10, 9, 19, 17, 21, 1, 20, 0, 16, 23, 7},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 22, 8, 13, 6, 27, 9, 19, 24, 21, 1, 20, 0, 16, 23, 26, 17, 10, 7, 25},
		{28, 3, 18, 5, 12, 29, 14, 11, 15, 22, 8, 13, 31, 27, 9, 19, 24, 21, 1, 20, 0, 16, 23, 26, 30, 10, 7, 25, 2, 4, 17, 6},
		{28, 3, 18, 5, 12, 29, 14, 11, 15, 22, 8, 13, 31, 27, 9, 19, 24, 21, 1, 20, 0, 16, 23, 26, 30, 10, 7, 25, 2, 4, 17, 6},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 22, 8, 13, 6, 27, 9, 19, 24, 21, 1, 20, 0, 16, 23, 26, 17, 10, 7, 25},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 22, 8, 13, 6, 10, 9, 19, 17, 21, 1, 20, 0, 16, 23, 7},
		{2, 3, 18, 5, 12, 4, 14, 11, 15, 7, 8, 13, 6, 10, 9, 19, 17, 16, 1, 0},
		{2, 3, 1, 5, 12, 4, 14, 11, 15, 7, 8, 13, 6, 10, 9, 0},
		{2, 3, 1, 5, 6, 4, 9, 11, 0, 7, 8, 10},
		{2, 3, 1, 5, 6, 4, 7, 0},
		{2, 3, 1, 0},
		{},
	}

	assert.Equal(t, expected, actual)
}
