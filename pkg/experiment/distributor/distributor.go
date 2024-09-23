package distributor

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/ring"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

const (
	defaultRingUpdateInterval = 5 * time.Second
	defaultLocationsPerKey    = 5
)

type Distributor struct {
	mu                 sync.RWMutex
	distribution       *distribution
	placement          placement.Strategy
	RingUpdateInterval time.Duration
	LocationsPerKey    int
}

func NewDistributor(placementStrategy placement.Strategy) *Distributor {
	return &Distributor{
		placement:          placementStrategy,
		RingUpdateInterval: defaultRingUpdateInterval,
		LocationsPerKey:    defaultLocationsPerKey,
	}
}

func (d *Distributor) Distribute(k placement.Key, r ring.ReadRing) (*placement.Placement, error) {
	if err := d.updateDistribution(r, d.RingUpdateInterval); err != nil {
		return nil, err
	}
	return d.distribute(k), nil
}

// TODO(kolesnikovae):
// Essentially, we do not need a ring. Instead, it would be better to access
// the memberlist/serf directly and build the distribution from there and
// generate tokens as needed. Or, alternatively, we could implement the
// BasicLifecyclerDelegate interface.

func (d *Distributor) updateDistribution(r ring.ReadRing, maxAge time.Duration) error {
	d.mu.RLock()
	x := d.distribution
	if x != nil && !x.isExpired(maxAge) {
		d.mu.RUnlock()
		return nil
	}
	d.mu.RUnlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	x = d.distribution
	if x != nil && !x.isExpired(maxAge) {
		return nil
	}

	// Initial capacity.
	const defaultShards = 64
	if x == nil {
		x = newDistribution(defaultShards)
	}

	if err := x.readRing(r); err != nil {
		return fmt.Errorf("failed to read ring: %w", err)
	}

	d.distribution = x
	return nil
}

func (d *Distributor) distribute(k placement.Key) *placement.Placement {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Determine the number of shards for the tenant within the available
	// space, and the dataset shards within the tenant subring.
	s := len(d.distribution.shards)
	p := min(d.LocationsPerKey, s)
	tenantSize := d.placement.NumTenantShards(k, s)
	if tenantSize == 0 || tenantSize > s {
		tenantSize = s
	}
	datasetSize := min(tenantSize, max(1, d.placement.NumDatasetShards(k, tenantSize)))
	// When we create subrings, we need to ensure that each of them has at
	// least p shards. However, the data distribution must be restricted
	// according to the limits.
	allShards := newSubring(s)
	tenantShards := allShards.subring(k.Tenant, max(p, tenantSize), s)
	datasetShards := tenantShards.subring(k.Dataset, max(p, datasetSize), tenantSize)
	// We pick a shard from the dataset subring: its index is relative
	// to the dataset subring.
	offset := d.placement.PickShard(k, datasetSize)
	// Next we want to find p instances eligible to host the key.
	// The choice must be limited to the dataset / tenant subring.
	// The iterator is used to iterate over the tenant instances that
	// can be used to host the key. In case if the instance is
	// unavailable, the caller should try the next one.
	loc := &location{
		d:    d.distribution,
		ring: datasetShards,
		off:  offset,
		n:    p,
	}
	return &placement.Placement{
		Shard:     loc.shard().id,
		Instances: loc.instances(),
	}
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

func newDistribution(shards int) *distribution {
	return &distribution{
		shards:    make([]shard, 0, shards),
		timestamp: time.Now(),
	}
}

func (d *distribution) isExpired(maxAge time.Duration) bool {
	return time.Now().Add(-maxAge).After(d.timestamp)
}

// TODO(kolesnikovae):
// Ensure we have access to all instances in the ring, regardless of their
// state. The ring exposes only healthy instances, which is not what we want,
// and this will lead to vast shard relocations and will deteriorate data
// locality if instances leave and join the ring frequently. Currently, the
// heartbeat timeout is set to 1m by default, which should prevent us from
// severe problems, but it's still a problem.
func (d *distribution) readRing(r ring.ReadRing) error {
	all, err := r.GetAllHealthy(ring.Write)
	if err != nil {
		return err
	}
	if len(all.Instances) == 0 {
		return ring.ErrEmptyRing
	}
	d.desc = all.Instances
	d.shards = d.shards[:0]
	slices.SortFunc(d.desc, func(a, b ring.InstanceDesc) int {
		return strings.Compare(a.Id, b.Id)
	})
	i := uint32(0)
	for j := range d.desc {
		for range all.Instances[j].Tokens {
			i++
			d.shards = append(d.shards, shard{
				id:       i,
				instance: uint32(j),
			})
		}
	}
	d.timestamp = time.Now()
	return nil
}

// The inputs are a key and the number of buckets.
// It outputs a bucket number in the range [0, buckets).
//
// Refer to https://arxiv.org/pdf/1406.2294:
// The function satisfies the two properties:
//  1. About the same number of keys map to each bucket.
//  2. The mapping from key to bucket is perturbed as little as possible when
//     the number of buckets is changed. Thus, the only data that needs to move
//     when the number of buckets changes is the data for the relatively small
//     number of keys whose bucket assignment changed.
func jump(key uint64, buckets int) int {
	var b, j = -1, 0
	for j < buckets {
		b = j
		key = key*2862933555777941757 + 1
		j = int(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}
	return b
}

// Subring is a utility to calculate the subring
// for a given key withing the available space:
//
// |<---------n----------->| Available space.
// | . a---|---------b . . | Ring.
// | . . . c-----d . . . . | Subring.
//
// [ . a-------|-----b . . ]
// [ . . . . . c-----|---d ]
//
// [ . a-------|-----b . . ]
// [ . . . . . c-----|-x-d ]
//
// [ . a-------|-----b . . ]
// [ . |-x-d . c-----| . . ]
//
// Note that this is not a recursive implementation,
// but a more straightforward one, optimized for the
// case where there can be up to two nested rings.
type subring struct {
	n, a, b, c, d int
	// For testing purposes jump function can be replaced.
	jump func(k uint64, n int) int
}

func newSubring(n int) subring { return subring{n: n, d: n, jump: jump} }

// The function creates a subring of the specified size for the given key.
// The subring offset is calculated with the jump function and is limited
// to m options (sequentially, from the ring beginning).
func (s subring) subring(k uint64, size, m int) subring {
	n := s
	n.a, n.b = n.c, n.d
	n.c = n.a + s.jump(k, min(m, n.b-n.a))
	n.d = n.c + size
	return n
}

// The function returns the absolute offset of the relative n.
func (s subring) at(n int) int {
	x := s.c + n
	x = (x - s.a) % (s.b - s.a)
	p := (x + s.a) % s.n
	return p
}

type location struct {
	d    *distribution
	ring subring
	off  int
	n    int
}

func (l *location) shard() shard {
	a := l.ring.at(l.off)
	return l.d.shards[a]
}

func (l *location) instances() iter.Iterator[ring.InstanceDesc] {
	instances := make([]ring.InstanceDesc, l.n)
	for i := 0; i < l.n; i++ {
		a := l.ring.at(l.off + i)
		s := l.d.shards[a]
		instances[i] = l.d.desc[s.instance]
	}
	return iter.NewSliceIterator(instances)
}
