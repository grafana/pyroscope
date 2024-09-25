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
	if p := d.placement.Place(k); p != nil {
		return p, nil
	}
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
	if x == nil {
		x = newDistribution()
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
	all := newSubring(s)
	tenant := all.subring(k.Tenant, tenantSize)
	dataset := tenant.subring(k.Dataset, datasetSize)
	// We pick a shard from the dataset subring: its index is relative
	// to the dataset subring.
	offset := d.placement.PickShard(k, datasetSize)
	// Next we want to find p instances eligible to host the key.
	// The choice must be limited to the dataset / tenant subring,
	// but extended if needed. Note that the instances are not unique.
	// We could collect instances lazily and pull them from the iterator,
	// however that would complicate the code due to concurrent updates.
	instances := make([]ring.InstanceDesc, 0, p)
	// Collect instances from the dataset subring.
	instances = d.distribution.collect(instances, dataset, offset, p)
	// Collect remaining instances from the tenant subring.
	instances = d.distribution.collect(instances, tenant, dataset.offset()+dataset.size(), p-len(instances))
	// Collect remaining instances from the top level ring.
	instances = d.distribution.collect(instances, all, tenant.offset()+tenant.size(), p-len(instances))
	return &placement.Placement{
		Shard:     uint32(dataset.at(offset)) + 1, // 0 shard ID is a sentinel.
		Instances: iter.NewSliceIterator(instances),
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
	d.timestamp = time.Now()
	d.desc = all.Instances
	// Jump consistent hashing requires a deterministic order of instances.
	// Moreover, instances can be only added to the end, otherwise this may
	// cause massive relocations.
	slices.SortFunc(d.desc, func(a, b ring.InstanceDesc) int {
		return strings.Compare(a.Id, b.Id)
	})
	// Now we create a mapping of shards to instances.
	var tmp [256]uint32 // Stack alloc.
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
	d.shards = slices.Grow(d.shards, max(0, size-len(d.shards)))[:size]
	for j := range d.shards {
		d.shards[j] = instances[d.perm.v[j]]
	}
	return nil
}

// collect n instances from the subring r starting from the offset off.
func (d *distribution) collect(instances []ring.InstanceDesc, r subring, off, n int) []ring.InstanceDesc {
	if n <= 0 {
		return instances
	}
	size := r.size()
	var added int
	for i := off; added < size && added < n; i++ {
		instances = append(instances, d.desc[d.shards[r.at(i)]])
		added++
	}
	return instances
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
// The subring offset is calculated with the jump function and is limited
// to m options (sequentially, from the ring beginning).
func (s subring) subring(k uint64, size int) subring {
	n := s
	n.a, n.b = n.c, n.d
	n.c = n.a + jump(k, n.b-n.a)
	n.d = n.c + size
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

func (s subring) offset() int { return s.c - s.a }

func (s subring) size() int { return s.d - s.c }

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
