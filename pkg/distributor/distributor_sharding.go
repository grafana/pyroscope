package distributor

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/ring"
)

var (
	distributionCache  sync.RWMutex
	cachedDistribution *distribution
)

const maxDistributionAge = time.Second * 5

func getDistribution(r ring.ReadRing, maxAge time.Duration) (*distribution, error) {
	distributionCache.RLock()
	d := cachedDistribution
	if d != nil && !d.isExpired(maxAge) {
		distributionCache.RUnlock()
		return d, nil
	}
	distributionCache.RUnlock()
	distributionCache.Lock()
	defer distributionCache.Unlock()
	if d != nil && !d.isExpired(maxAge) {
		return d, nil
	}
	var shards = 64
	var instances = 128
	if d != nil {
		shards = len(d.shards)
		instances = len(d.desc)
	}
	n := newDistribution(shards, instances)
	if err := n.readRing(r); err != nil {
		return nil, fmt.Errorf("failed to read ring: %w", err)
	}
	cachedDistribution = n
	return n, nil
}

type distribution struct {
	timestamp time.Time
	shards    []shard
	desc      []ring.InstanceDesc
}

type shard struct {
	id       uint32 // 0 shard ID is used as a sentinel (zero value is invalid).
	instance uint32 // references the instance in shards.desc.
}

func newDistribution(shards, instances int) *distribution {
	return &distribution{
		shards:    make([]shard, 0, shards),
		desc:      make([]ring.InstanceDesc, 0, instances),
		timestamp: time.Now(),
	}
}

func (d *distribution) isExpired(maxAge time.Duration) bool {
	return time.Now().Add(-maxAge).After(d.timestamp)
}

func (d *distribution) readRing(r ring.ReadRing) error {
	all, err := r.GetAllHealthy(ring.Write)
	if err != nil {
		return err
	}
	if len(all.Instances) == 0 {
		return ring.ErrEmptyRing
	}
	d.desc = all.Instances
	// Jump hashing needs order.
	slices.SortFunc(d.desc, func(a, b ring.InstanceDesc) int {
		return strings.Compare(a.Id, b.Id)
	})
	i := uint32(0)
	for j := range all.Instances {
		for range all.Instances[j].Tokens {
			i++
			d.shards = append(d.shards, shard{
				id:       i,
				instance: uint32(j),
			})
		}
	}
	return nil
}

// The constant determines which keys are generated for the
// jump hashing function. A generated value is added to the
// tenant service key hash to produce the next jump hashing
// key. The seed is fixed to ensure deterministic behaviour
// across instances. The value is a random generated with a
// crypto/rand.Read, and decoded as a little-endian uint64.
const serviceShardsRandSeed = 4349676827832284783

func (d *distribution) serviceShards(n int, service uint64) []shard {
	rnd := rand.New(rand.NewSource(serviceShardsRandSeed))
	m := len(d.shards)
	if m < n {
		n = m
	}
	s := make([]shard, 0, n)
	for i := 0; i < n; i++ {
		j := jump(service&^rnd.Uint64(), m)
		s = append(s, d.shards[j])
	}
	return s
}

// https://arxiv.org/pdf/1406.2294
func jump(key uint64, buckets int) int {
	var b, j = -1, 0
	for j < buckets {
		b = j
		key = key*2862933555777941757 + 1
		j = int(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}
	return b
}
