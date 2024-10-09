package adaptive_placement

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
)

func Test_loadBalancingStrategy(t *testing.T) {
	rnd := rand.New(rand.NewSource(randSeed))
	const unitSize = 512 << 10

	randomize := func(f float64, values ...uint64) []uint64 {
		for i, v := range values {
			j := uint64(float64(v) * f)
			if rnd.Float64() > 0.5 {
				values[i] += j
			} else {
				values[i] -= j
			}
		}
		return values
	}

	for i, test := range []struct {
		usage    []uint64
		expected LoadBalancing
	}{
		{
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{0},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{unitSize},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{0, 0, 0, 0, 0},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{unitSize, unitSize, unitSize, unitSize, unitSize},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{2 * unitSize, unitSize, unitSize, unitSize, unitSize},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    randomize(0.1, unitSize, unitSize, unitSize, unitSize, unitSize),
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    randomize(0.9, unitSize, unitSize, unitSize, unitSize, unitSize),
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    randomize(0.1, 2*unitSize, 2*unitSize, 2*unitSize, 2*unitSize, 2*unitSize),
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{2 * unitSize, unitSize / 2, unitSize, unitSize, unitSize},
			expected: FingerprintLoadBalancing,
		},
		{
			usage:    []uint64{2 * unitSize, unitSize / 2, unitSize / 2, unitSize, unitSize},
			expected: RoundRobinLoadBalancing,
		},
		{
			usage:    randomize(0.9, 2*unitSize, 2*unitSize, 2*unitSize, 2*unitSize, 2*unitSize),
			expected: RoundRobinLoadBalancing,
		},
	} {
		stats := &adaptive_placementpb.DatasetStats{
			Shards: make([]uint32, len(test.usage)),
			Usage:  test.usage,
			StdDev: stdDev(test.usage),
		}
		target := len(stats.Shards)
		assert.Equal(t, test.expected, loadBalancingStrategy(stats, unitSize, target), fmt.Sprint(i))
	}

}

func Test_loadBalancingStrategy_relocation(t *testing.T) {
	const unitSize = 512 << 10
	for i, test := range []struct {
		usage    []uint64
		expected LoadBalancing
		target   int
	}{
		{
			usage:    []uint64{2 * unitSize, 2 * unitSize, unitSize / 2, unitSize / 2, unitSize / 2},
			expected: RoundRobinLoadBalancing,
			target:   5, // 5/5
		},
		{
			usage:    []uint64{2 * unitSize, 2 * unitSize, unitSize / 2, unitSize / 2},
			expected: RoundRobinLoadBalancing,
			target:   2, // 2/4
		},
		{
			usage:    []uint64{2 * unitSize, 2 * unitSize, unitSize / 2, unitSize / 2, unitSize / 2},
			expected: RoundRobinLoadBalancing,
			target:   3, // 3/5
		},
		{
			usage:    []uint64{2 * unitSize, 2 * unitSize, unitSize / 2, unitSize / 2, unitSize / 2},
			expected: FingerprintLoadBalancing,
			target:   2, // 2/5
		},
		{
			usage:    []uint64{unitSize, unitSize, unitSize / 5, unitSize / 5},
			expected: FingerprintLoadBalancing,
			target:   2, // 2/4
		},
		{
			usage:    []uint64{2*unitSize - 1, 0},
			expected: FingerprintLoadBalancing,
			target:   1, // 1/2
		},
	} {
		stats := &adaptive_placementpb.DatasetStats{
			Shards: make([]uint32, len(test.usage)),
			Usage:  test.usage,
			StdDev: stdDev(test.usage),
		}
		assert.Equal(t, test.expected, loadBalancingStrategy(stats, unitSize, test.target), fmt.Sprint(i))
	}
}
