package distributor

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockplacement"
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
		{Id: "a", Tokens: make([]uint32, 1)},
		{Id: "b", Tokens: make([]uint32, 1)},
		{Id: "c", State: ring.LEAVING, Tokens: make([]uint32, 1)},
	}
	zeroShard = func(int) int { return 0 }
)

func Test_EmptyRing(t *testing.T) {
	m := new(mockplacement.MockPlacement)
	r := testhelper.NewMockRing(nil, 1)
	d := NewDistributor(m, r)

	k := NewTenantServiceDatasetKey("")
	_, err := d.Distribute(k)
	assert.ErrorIs(t, err, ring.ErrEmptyRing)
}

func Test_Distribution_AvailableShards(t *testing.T) {
	for _, tc := range []struct {
		description string
		placement.Policy
	}{
		{
			description: "zero",
			Policy: placement.Policy{
				TenantShards:  0,
				DatasetShards: 0,
				PickShard:     zeroShard,
			},
		},
		{
			description: "min",
			Policy: placement.Policy{
				TenantShards:  1,
				DatasetShards: 1,
				PickShard:     zeroShard,
			},
		},
		{
			description: "insufficient",
			Policy: placement.Policy{
				TenantShards:  1 << 10,
				DatasetShards: 1 << 9,
				PickShard:     zeroShard,
			},
		},
		{
			description: "invalid",
			Policy: placement.Policy{
				TenantShards:  1 << 10,
				DatasetShards: 2 << 10,
				PickShard:     zeroShard,
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			k := NewTenantServiceDatasetKey("tenant-a", testLabels...)
			m := new(mockplacement.MockPlacement)
			m.On("Policy", k, mock.Anything).Return(tc.Policy).Once()
			r := testhelper.NewMockRing(testInstances, 1)
			d := NewDistributor(m, r)
			p, err := d.Distribute(k)
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
	m := new(mockplacement.MockPlacement)
	m.On("Policy", k, mock.Anything).Return(placement.Policy{
		TenantShards:  1,
		DatasetShards: 1,
		PickShard:     zeroShard,
	})

	r := testhelper.NewMockRing(testInstances, 1)
	d := NewDistributor(m, r)
	_, err := d.Distribute(k)
	require.NoError(t, err)

	instances := make([]ring.InstanceDesc, 2)
	copy(instances, testInstances[:1])
	r.SetInstances(instances)
	require.NoError(t, d.updateDistribution(r, 0))

	p, err := d.Distribute(k)
	require.NoError(t, err)
	c := make([]ring.InstanceDesc, 0, 1)
	for p.Instances.Next() {
		c = append(c, p.Instances.At())
	}

	// Only one instance is available.
	assert.Equal(t, 1, len(c))
	m.AssertExpectations(t)
}

func Test_Distributor_Distribute(t *testing.T) {
	m := new(mockplacement.MockPlacement)
	r := testhelper.NewMockRing([]ring.InstanceDesc{
		{Id: "a", Tokens: make([]uint32, 4)},
		{Id: "b", Tokens: make([]uint32, 4)},
		{Id: "c", Tokens: make([]uint32, 4)},
	}, 1)

	d := NewDistributor(m, r)
	collect := func(offset, n int) []string {
		h := uint64(14046587775414411003)
		k := placement.Key{
			Tenant:      h,
			Dataset:     h,
			Fingerprint: h,
		}
		m.On("Policy", k).Return(placement.Policy{
			TenantShards:  8,
			DatasetShards: 4,
			PickShard:     func(int) int { return offset },
		}).Once()
		p, err := d.Distribute(k)
		require.NoError(t, err)
		return collectN(p.Instances, n)
	}

	//   0 1 2 3 4 5 6 7 8 9 10 11  all shards
	//   * * * *         > * *  *   tenant (size 8, offset 8)
	//       > *         * *        dataset (size 4, offset 6+8 mod 12 = 2)
	//   a a a b b b c c a b c  c   shuffling (see d.distribution.shards)
	//   ----------------------------------------------------------------------
	//       0 1         2 3 4      PickShard 0 (offset within dataset)
	//                       ^      borrowed from the tenant
	//
	//       3 0         1 2 4      PickShard 1
	//       2 3         0 1 4      PickShard 2
	//       1 2         3 0 4      PickShard 3

	// Identical keys have identical placement.
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(0, 5))
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(0, 5))

	// Placement of different keys in the dataset is bound.
	assert.Equal(t, []string{"b", "a", "b", "a", "c"}, collect(1, 5))
	assert.Equal(t, []string{"a", "b", "a", "b", "c"}, collect(2, 5))
	assert.Equal(t, []string{"b", "a", "b", "a", "c"}, collect(3, 5))

	// Now we're trying to collect more instances than available.
	//   0 1 2 3 4 5 6  7  8 9 10 11  all shards
	//   * * * *           > * *  *   tenant (size 8, offset 8)
	//       > *           x *        dataset (size 4, offset 6+8 mod 12 = 2)
	//       0 1           2          PickShard 2 (13)
	//   6 7 2 3 8 9 10 11 0 1 4  5
	//   ^ ^                   ^  ^   borrowed from the tenant
	//           ^ ^ ^  ^             borrowed from the top ring
	//   a a a b b b c  c  a b c  c   shuffling (see d.distribution.shards)
	assert.Equal(t, []string{"a", "b", "a", "b", "c", "c", "a", "a", "b", "b", "c", "c"}, collect(2, 13))
}

func Test_distribution_iterator(t *testing.T) {
	d := &distribution{
		shards: []uint32{0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2},
		desc:   []ring.InstanceDesc{{Id: "a"}, {Id: "b"}, {Id: "c"}},
	}

	t.Run("empty ring", func(t *testing.T) {
		assert.Equal(t, []string{}, collectN(d.instances(subring{}, 0), 10))
	})

	t.Run("matching subrings", func(t *testing.T) {
		r := subring{
			n: 12,
			a: 8,
			b: 16,
			c: 8,
			d: 16,
		}

		//   0 1 2 3 4 5 6  7  8 9 10 11  all shards
		//   a a a a b b b  b  c c c  c   no shuffling (!)
		//   * * * *           > * *  *   tenant (size 8, offset 8)
		//   * * * *           > * *  *   dataset (size 8, offset 8)
		//
		//   4 5 6 7|8 9 10 11|0 1 2  3   PickShard 0 (offset within dataset/tenant)
		//   3 4 5 6|8 9 10 11|7 0 1  2   PickShard 1
		//   2 3 4 5|8 9 10 11|6 7 0  1   PickShard 2

		var expected bytes.Buffer
		for _, line := range []string{
			"0 [c c c c a a a a b b b b]",
			"1 [c c c a a a a c b b b b]",
			"2 [c c a a a a c c b b b b]",
			"3 [c a a a a c c c b b b b]",
			"4 [a a a a c c c c b b b b]",
			"5 [a a a c c c c a b b b b]",
			"6 [a a c c c c a a b b b b]",
			"7 [a c c c c a a a b b b b]",
			"8 [c c c c a a a a b b b b]",
			"9 [c c c a a a a c b b b b]",
		} {
			_, _ = fmt.Fprintln(&expected, line)
		}

		var actual bytes.Buffer
		for i := 0; i < 10; i++ {
			_, _ = fmt.Fprintln(&actual, i, collectN(d.instances(r, i), 20))
		}

		assert.Equal(t, expected.String(), actual.String())
	})

	t.Run("nested subrings", func(t *testing.T) {
		r := subring{
			n: 12,
			a: 1,
			b: 9,
			c: 3,
			d: 7,
		}

		//   0  1  2 3 4 5 6 7 8 9 10 11  all shards
		//   a  a  a a b b b b c c c  c   no shuffling (!)
		//      >  * * * * * * *          tenant (size 8, offset 1)
		//           > * * *              dataset (size 4, offset 3)
		//
		//   11 4  5 0 1 2 3 6 7 8 9  10  PickShard 0 (offset within dataset)
		//   11 4  5 3 0 1 2 6 7 8 9  10  PickShard 1

		var expected bytes.Buffer
		for _, line := range []string{
			"0 [a b b b b c a a c c c a]",
			"1 [b b b a b c a a c c c a]",
			"2 [b b a b b c a a c c c a]",
			"3 [b a b b b c a a c c c a]",
			"4 [a b b b b c a a c c c a]",
			"5 [b b b a b c a a c c c a]",
			"6 [b b a b b c a a c c c a]",
			"7 [b a b b b c a a c c c a]",
			"8 [a b b b b c a a c c c a]",
			"9 [b b b a b c a a c c c a]",
		} {
			_, _ = fmt.Fprintln(&expected, line)
		}

		var actual bytes.Buffer
		for i := 0; i < 10; i++ {
			_, _ = fmt.Fprintln(&actual, i, collectN(d.instances(r, i), 20))
		}

		assert.Equal(t, expected.String(), actual.String())
	})

	t.Run("nested subrings aligned", func(t *testing.T) {
		r := subring{
			n: 12,
			a: 1,
			b: 9,
			c: 1,
			d: 5,
		}

		//   0  1 2 3 4 5 6 7 8 9 10 11  all shards
		//   a  a a a b b b b c c c  c   no shuffling (!)
		//      > * * * * * * *          tenant (size 8, offset 1)
		//      > * * *                  dataset (size 4, offset 1)

		var expected bytes.Buffer
		for _, line := range []string{
			"0 [a a a b b b b c c c c a]",
			"1 [a a b a b b b c c c c a]",
			"2 [a b a a b b b c c c c a]",
			"3 [b a a a b b b c c c c a]",
			"4 [a a a b b b b c c c c a]",
			"5 [a a b a b b b c c c c a]",
			"6 [a b a a b b b c c c c a]",
			"7 [b a a a b b b c c c c a]",
			"8 [a a a b b b b c c c c a]",
			"9 [a a b a b b b c c c c a]",
		} {
			_, _ = fmt.Fprintln(&expected, line)
		}

		var actual bytes.Buffer
		for i := 0; i < 10; i++ {
			_, _ = fmt.Fprintln(&actual, i, collectN(d.instances(r, i), 20))
		}

		assert.Equal(t, expected.String(), actual.String())
	})

	t.Run("nested subrings wrap", func(t *testing.T) {
		r := subring{
			n: 12,
			a: 8,
			b: 16,
			c: 10,
			d: 14,
		}

		//   0 1 2 3 4 5 6 7 8 9 10 11  all shards
		//   a a a a b b b b c c c  c   no shuffling (!)
		//   * * * *         > * *  *   tenant (size 8, offset 8)
		//   * *                 >  *   dataset (size 4, offset 14 mod 12 = 2)

		var expected bytes.Buffer
		for _, line := range []string{
			"0 [c c a a a a c c b b b b]",
			"1 [c a a c a a c c b b b b]",
			"2 [a a c c a a c c b b b b]",
			"3 [a c c a a a c c b b b b]",
			"4 [c c a a a a c c b b b b]",
			"5 [c a a c a a c c b b b b]",
			"6 [a a c c a a c c b b b b]",
			"7 [a c c a a a c c b b b b]",
			"8 [c c a a a a c c b b b b]",
			"9 [c a a c a a c c b b b b]",
		} {
			_, _ = fmt.Fprintln(&expected, line)
		}

		var actual bytes.Buffer
		for i := 0; i < 10; i++ {
			_, _ = fmt.Fprintln(&actual, i, collectN(d.instances(r, i), 20))
		}

		assert.Equal(t, expected.String(), actual.String())
	})

	t.Run("overlapping subrings", func(t *testing.T) {
		r := subring{
			n: 12,
			a: 8,
			b: 16,
			c: 14,
			d: 18,
		}

		//   0 1 2 3 4 5 6  7  8 9 10 11  all shards
		//   a a a a b b b  b  c c c  c   no shuffling (!)
		//   * * * *           > * *  *   tenant (size 8, offset 8)
		//       > *           * *        dataset (size 4, offset 14 mod 12 = 2)
		//   6 7|0 1|8 9 10 11|2 3|4  5   PickShard 0 (offset within dataset)
		//   6 7|3 0|8 9 10 11|1 2|4  5   PickShard 1
		//   6 7|2 3|8 9 10 11|0 1|4  5   PickShard 2
		//   6 7|1 2|8 9 10 11|3 0|4  5   PickShard 3

		var expected bytes.Buffer
		for _, line := range []string{
			"0 [a a c c c c a a b b b b]",
			"1 [a c c a c c a a b b b b]",
			"2 [c c a a c c a a b b b b]",
			"3 [c a a c c c a a b b b b]",
			"4 [a a c c c c a a b b b b]",
			"5 [a c c a c c a a b b b b]",
			"6 [c c a a c c a a b b b b]",
			"7 [c a a c c c a a b b b b]",
			"8 [a a c c c c a a b b b b]",
			"9 [a c c a c c a a b b b b]",
		} {
			_, _ = fmt.Fprintln(&expected, line)
		}

		var actual bytes.Buffer
		for i := 0; i < 10; i++ {
			_, _ = fmt.Fprintln(&actual, i, collectN(d.instances(r, i), 20))
		}

		assert.Equal(t, expected.String(), actual.String())
	})
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

func collectN(i iter.Iterator[ring.InstanceDesc], n int) []string {
	s := make([]string, 0, n)
	for n > 0 && i.Next() {
		s = append(s, i.At().Id)
		n--
	}
	return s
}
