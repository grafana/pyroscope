package distributor

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/ring"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
)

const defaultRingUpdateInterval = 5 * time.Second

type Distributor struct {
	mu                 sync.RWMutex
	distribution       *distribution
	placement          placement.Strategy
	RingUpdateInterval time.Duration
}

func NewDistributor(placementStrategy placement.Strategy) *Distributor {
	return &Distributor{
		placement:          placementStrategy,
		RingUpdateInterval: defaultRingUpdateInterval,
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
	ts := d.placement.NumTenantShards(k, len(d.distribution.shards))
	if ts == 0 || ts > len(d.distribution.shards) {
		ts = len(d.distribution.shards)
	}
	ds := min(ts, max(1, d.placement.NumDatasetShards(k, ts)))
	s := newSubring(len(d.distribution.shards))
	s = s.subring(k.Tenant, ts)
	s = s.subring(k.Dataset, ds)
	n := d.distribution.shards[s.at(d.placement.PickShard(k, ds))]
	x := uint32(d.distribution.perm.v[n.id-1])
	return &placement.Placement{Shard: x}
}

type distribution struct {
	timestamp time.Time
	shards    []shard
	desc      []ring.InstanceDesc
	perm      *perm
}

type shard struct {
	id       uint32 // 0 shard ID is used as a sentinel (zero value is invalid).
	instance uint32 // references the instance in shards.desc.
}

func newDistribution(shards int) *distribution {
	return &distribution{
		shards:    make([]shard, 0, shards),
		timestamp: time.Now(),
		perm:      new(perm),
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
	d.perm.resize(int(i))
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
type subring struct{ n, a, b, c, d int }

func newSubring(n int) subring { return subring{n: n, d: n} }

func (s subring) subring(k uint64, size int) subring {
	n := s
	n.a, n.b = n.c, n.d
	n.c = n.a + jump(k, n.b-n.a)
	n.d = n.c + size
	return n
}

func (s subring) at(n int) int {
	x := s.c + n
	x = (x - s.a) % (s.b - s.a)
	p := (x + s.a) % s.n
	return p
}

// The value is a random generated with a crypto/rand.Read,
// and decoded as a little-endian uint64. No fancy math here.
const randSeed = 4349576827832984783

var rnd = rand.New(rand.NewSource(randSeed))

// perm is a utility to generate deterministic permutations
// with a fixed seed. The permutation is used to shuffle the
// shards: when the number of shard changes, only affected
// permutations are updated.
type perm struct{ v, i []int }

func (p *perm) resize(n int) {
	if len(p.v) == n {
		return
	}
	if n <= 0 {
		p.v = p.v[:0]
		p.i = p.i[:0]
		return
	}
	if len(p.v) < n {
		// Fisher–Yates shuffle.
		s := len(p.v)
		p.v = slices.Grow(p.v, n-s)[:n]
		p.i = slices.Grow(p.i, n-s)[:n]
		for i := s; i < n; i++ {
			j := rnd.Intn(i + 1)
			p.v[i] = p.v[j]
			p.v[j] = i
		}
	} else {
		// Reverse permutations.
		for i := len(p.v) - 1; i >= n; i-- {
			x := p.i[i]
			p.i[p.v[i]] = x
			p.v[x] = p.v[i]
		}
		p.v = p.v[:n]
	}
	// Update all indices.
	for i, v := range p.v {
		p.i[v] = i
	}
	p.i = p.i[:n]
}
