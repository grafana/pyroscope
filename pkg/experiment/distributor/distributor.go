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

// NOTE(kolesnikovae): Essentially, we do not depend on the dskit/ring and
// only use it as a discovery mechanism build on top of the memberlist.
// It would be better to access the memberlist/serf directly.
var op = ring.NewOp([]ring.InstanceState{ring.ACTIVE, ring.LEAVING, ring.PENDING, ring.JOINING}, nil)

const defaultRingUpdateInterval = 5 * time.Second

type Distributor struct {
	mu           sync.RWMutex
	ring         ring.ReadRing
	placement    placement.Placement
	distribution *distribution

	RingUpdateInterval time.Duration
}

func NewDistributor(placement placement.Placement, r ring.ReadRing) *Distributor {
	return &Distributor{
		ring:               r,
		placement:          placement,
		RingUpdateInterval: defaultRingUpdateInterval,
	}
}

func (d *Distributor) Distribute(k placement.Key) (*placement.ShardMapping, error) {
	if err := d.updateDistribution(d.ring, d.RingUpdateInterval); err != nil {
		return nil, err
	}
	return d.distribute(k), nil
}

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
	if x == nil {
		x = newDistribution()
	}
	if err := x.readRing(r); err != nil {
		return fmt.Errorf("failed to read ring: %w", err)
	}
	d.distribution = x
	return nil
}

func (d *Distributor) distribute(k placement.Key) *placement.ShardMapping {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Determine the number of shards for the tenant within the available
	// space, and the dataset shards within the tenant subring.
	s := len(d.distribution.shards)
	p := d.placement.Policy(k)
	tenantSize := p.TenantShards
	if tenantSize == 0 || tenantSize > s {
		tenantSize = s
	}
	datasetSize := min(tenantSize, max(1, p.DatasetShards))
	// When we create subrings, we need to ensure that each of them has at
	// least p shards. However, the data distribution must be restricted
	// according to the limits.
	all := newSubring(s)
	tenant := all.subring(k.Tenant, tenantSize)
	dataset := tenant.subring(k.Dataset, datasetSize)
	// We pick a shard from the dataset subring: its index is relative
	// to the dataset subring.
	offset := p.PickShard(datasetSize)
	// Next we want to find p instances eligible to host the key.
	// The choice must be limited to the dataset / tenant subring,
	// but extended if needed.
	return &placement.ShardMapping{
		Shard:     uint32(dataset.at(offset)) + 1, // 0 shard ID is a sentinel
		Instances: d.distribution.instances(dataset, offset),
	}
}

type distribution struct {
	timestamp time.Time
	shards    []uint32 // Shard ID -> Instance ID.
	desc      []ring.InstanceDesc
	perm      *perm
}

func newDistribution() *distribution {
	return &distribution{
		timestamp: time.Now(),
		perm:      new(perm),
	}
}

func (d *distribution) isExpired(maxAge time.Duration) bool {
	return time.Now().Add(-maxAge).After(d.timestamp)
}

func (d *distribution) readRing(r ring.ReadRing) error {
	all, err := r.GetAllHealthy(op)
	if err != nil {
		return err
	}
	if len(all.Instances) == 0 {
		return ring.ErrEmptyRing
	}
	d.timestamp = time.Now()
	d.desc = all.Instances
	// Jump consistent hashing requires a deterministic order of instances.
	// Moreover, instances can be only added to the end, otherwise this may
	// cause massive relocations.
	slices.SortFunc(d.desc, func(a, b ring.InstanceDesc) int {
		return strings.Compare(a.Id, b.Id)
	})
	// Now we create a mapping of shards to instances.
	var tmp [256]uint32 // Try to allocate on stack.
	instances := tmp[:0]
	for j := range d.desc {
		for range all.Instances[j].Tokens {
			instances = append(instances, uint32(j))
		}
	}
	// We use shuffling to avoid hotspots: a contiguous range of shards
	// is distributed over instances in a pseudo-random fashion.
	// Given that the number of shards and instances is known in advance,
	// we maintain a deterministic permutation that perturbs as little as
	// possible, when the number of shards or instances changes: only the
	// delta moves.
	size := len(instances)
	d.perm.resize(size)
	// Note that we can't reuse d.shards because it may be used by iterators.
	// In fact, this is a snapshot that must not be modified.
	d.shards = make([]uint32, size)
	for j := range d.shards {
		d.shards[j] = instances[d.perm.v[j]]
	}
	return nil
}

// instances returns an iterator that iterates over instances
// that may host the shard at the offset in the order of preference:
// dataset -> tenant -> all shards.
func (d *distribution) instances(r subring, off int) *iterator {
	return &iterator{
		off:    off,
		lim:    r.size(),
		ring:   r,
		shards: d.shards,
		desc:   d.desc,
	}
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
// Note that this is not a recursive implementation,
// but a more straightforward one, optimized for the
// case where there can be up to two nested rings.
type subring struct {
	// |<---------n----------->| Available space.
	// | . a---|---------b . . | Ring.
	// | . . . c-----d . . . . | Subring.
	n, a, b, c, d int
}

func newSubring(n int) subring { return subring{n: n, b: n, d: n} }

// The function creates a subring of the specified size for the given key.
// The subring offset is calculated with the jump function.
func (s subring) subring(k uint64, size int) subring {
	n := s
	n.a, n.b = n.c, n.d
	n.c = n.a + jump(k, n.b-n.a)
	n.d = n.c + size
	return n
}

func (s subring) pop() subring {
	n := s
	n.c, n.d = n.a, n.b
	n.a, n.b = 0, n.n
	return n
}

// The function returns the absolute offset of the relative n.
func (s subring) at(n int) int {
	// [ . a-------|-----b . . ]
	// [ . . . . . c-----|-x-d ]
	//
	// [ . a-------|-----b . . ]
	// [ . |-x-d . c-----| . . ]
	n %= s.d - s.c
	x := s.c + n
	x = (x - s.a) % (s.b - s.a)
	p := (x + s.a) % s.n
	return p
}

// offset reports offset in the parent ring.
func (s subring) offset() int { return s.c - s.a }

// size reports the size of the ring.
func (s subring) size() int { return s.d - s.c }

// iterator iterates instances that host the shards of the subring.
// The iterator is not limited to the subring, and will continue with
// the parent subring when the current one is exhausted.
type iterator struct {
	n   int // Number of instances collected.
	off int // Current offset in the ring (relative).
	lim int // Remaining instances in the subring.

	ring   subring
	shards []uint32
	desc   []ring.InstanceDesc
}

func (i *iterator) Err() error { return nil }

func (i *iterator) Close() error { return nil }

func (i *iterator) Next() bool {
	if i.n >= i.ring.n {
		return false
	}
	if i.lim > 0 {
		i.lim--
	} else {
		for i.lim <= 0 {
			// We have exhausted the subring.
			// Navigate to the parent ring.
			if i.ring.n == i.ring.size() {
				// No parent rings left.
				return false
			}
			// Start with the offset right after the subring.
			size := i.ring.size()
			i.off = i.ring.offset() + size
			p := i.ring.pop() // Load parent.
			// How many items remain in the ring.
			i.lim = p.size() - size - 1
			i.ring = p
		}
	}
	i.off++
	i.n++
	return true
}

func (i *iterator) At() ring.InstanceDesc {
	a := i.ring.at(i.off - 1) // Translate the relative offset to absolute.
	x := i.shards[a]          // Map the shard to the instance.
	return i.desc[x]
}

// Fisher–Yates shuffle with predefined steps.
// Rand source with a seed is not enough as we
// can't guarantee the same sequence of calls
// with identical arguments, which would make
// the state of two instances incoherent.
type perm struct{ v []uint32 }

func (p *perm) resize(n int) {
	d := max(0, n-len(p.v))
	p.v = slices.Grow(p.v, d)[:n]
	// We do want to start with 0 (in contrast to the standard
	// implementation) as this is required for the n == 1 case:
	// we need to zero v[0].
	// Although, it's possible to make the change incrementally,
	// for simplicity, we just rebuild the permutation.
	for i := 0; i < n; i++ {
		j := steps[i]
		p.v[i], p.v[j] = p.v[j], uint32(i)
	}
}

// The value is a random generated with a crypto/rand.Read,
// and decoded as a little-endian uint64. No fancy math here.
const randSeed = 4349576827832984783

var steps [4 << 10]uint32

func init() {
	r := rand.New(rand.NewSource(randSeed))
	for i := range steps {
		steps[i] = uint32(r.Intn(i + 1))
	}
}
