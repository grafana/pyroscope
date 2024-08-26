package distributor

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/ring"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor/placement"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

const (
	defaultRingUpdateInterval = 5 * time.Second
	defaultFallbackLocations  = 5
)

// NewTenantServiceDatasetKey build a distribution key, where
func NewTenantServiceDatasetKey(tenant string, labels []*v1.LabelPair) placement.Key {
	dataset := phlaremodel.Labels(labels).Get(phlaremodel.LabelNameServiceName)
	return placement.Key{
		TenantID:    tenant,
		DatasetName: dataset,

		Tenant:      fnv64(tenant),
		Dataset:     fnv64(tenant, dataset),
		Fingerprint: fnv64(phlaremodel.LabelPairsString(labels)),
	}
}

func fnv64(keys ...string) uint64 {
	h := fnv.New64a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
	}
	return h.Sum64()
}

type Distributor struct {
	mu           sync.RWMutex
	distribution *distribution
	placement    placement.Strategy

	RingUpdateInterval time.Duration
	FallbackLocations  uint32
}

func NewDistributor(placementStrategy placement.Strategy) *Distributor {
	return &Distributor{
		placement: placementStrategy,

		RingUpdateInterval: defaultRingUpdateInterval,
		FallbackLocations:  defaultFallbackLocations,
	}
}

func (d *Distributor) Distribute(k placement.Key, r ring.ReadRing) (*placement.Placement, error) {
	if err := d.getDistribution(r, d.RingUpdateInterval); err != nil {
		return nil, err
	}
	// scope is a slice of shards that belong to the tenant dataset.
	// It might be larger than the actual number of shards allowed for use.
	// In case of a delivery failure, at least 5 options of the shard
	// placement (locations) are available.
	scope, size := d.datasetShards(k, d.FallbackLocations)
	// Next, we need to find the exact shard for the key from N options and
	// find instances where the shard may be placed.
	// Limit the scope for selection to the actual number of shards, allowed
	// for the tenant dataset.
	i := d.placement.PickShard(k, min(size, uint32(len(scope))))
	s := scope[i]
	// Collect all the instances available in the scope.
	// Note that the order of instances is deterministic.
	instances := make([]*ring.InstanceDesc, len(scope))
	for j, o := range scope {
		instances[j] = &d.distribution.desc[o.instance]
	}
	// Move the instance that owns the selected shard to the queue front;
	// other instances are only considered if the primary one is unavailable.
	instances[0], instances[i] = instances[i], instances[0]
	p := &placement.Placement{
		Instances: instances,
		Shard:     s.id,
	}
	return p, nil
}

// Update forcibly updates the distribution.
func (d *Distributor) Update(r ring.ReadRing) error {
	return d.getDistribution(r, 0)
}

// tenantShards returns the list of shards that are available to the tenant dataset.
func (d *Distributor) datasetShards(k placement.Key, ms uint32) ([]shard, uint32) {
	shards := d.tenantShards(k, ms)
	s := uint32(len(shards))
	size := d.placement.NumDatasetShards(k, s)
	if size == 0 {
		size = s
	}
	return d.distribution.selectShards(shards, max(size, ms), k.Dataset), size
}

// tenantShards returns the list of shards that are available to the tenant.
func (d *Distributor) tenantShards(k placement.Key, ms uint32) []shard {
	s := uint32(len(d.distribution.shards))
	size := d.placement.NumTenantShards(k, s)
	if size == 0 {
		return d.distribution.shards
	}
	return d.distribution.selectShards(nil, max(size, ms), k.Tenant)
}

// TODO(kolesnikovae):
// Essentially, we do not need a ring. Instead, it would be better to access
// the memberlist/serf directly and build the distribution from there and
// generate tokens as needed. Or, alternatively, we could implement the
// BasicLifecyclerDelegate interface.
func (d *Distributor) getDistribution(r ring.ReadRing, maxAge time.Duration) error {
	d.mu.RLock()
	x := d.distribution
	if x != nil && !x.isExpired(maxAge) {
		d.mu.RUnlock()
		return nil
	}
	d.mu.RUnlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if x != nil && !x.isExpired(maxAge) {
		return nil
	}

	// Initial capacity.
	var shards = 64
	var instances = 128
	if x != nil {
		shards = len(x.shards)
		instances = len(x.desc)
	}

	n := newDistribution(shards, instances)
	if err := n.readRing(r); err != nil {
		return fmt.Errorf("failed to read ring: %w", err)
	}

	d.distribution = n
	return nil
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
// tenant dataset key hash to produce the next jump hashing
// key. The seed is fixed to ensure deterministic behaviour
// across instances. The value is a random generated with a
// crypto/rand.Read, and decoded as a little-endian uint64.
const stepRandSeed = 4349676827832284783

// For a given key, we need to deterministically select N shards.
// The array stores precalculated jump hashing steps.
var steps [1024]uint64

func init() {
	rnd := rand.New(rand.NewSource(stepRandSeed))
	for i := range steps {
		steps[i] = rnd.Uint64()
	}
}

func (d *distribution) selectShards(s []shard, n uint32, k uint64) []shard {
	// m options are available total.
	m := len(d.shards)
	// pick n options from m.
	n = min(uint32(len(steps)), uint32(m), n)
	s = slices.Grow(s[:0], int(n))
	// Note that the choice is deterministic.
	for i := uint32(0); i < n; i++ {
		j := jump(k&^steps[i], m)
		s = append(s, d.shards[j])
	}
	return s
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
