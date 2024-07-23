package distributor

import (
	"fmt"

	"github.com/grafana/dskit/ring"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// FIXME(kolesnikovae):
//  1. Essentially, we do not need dskit ring. Instead, it would be better to access
//     the memberlist/serf directly and build the distribution from there (generating
//     tokens as we want). Or, alternatively, we could implement BasicLifecyclerDelegate
//     interface.
//  2. Ensure we have access to all ingester instances, regardless of their state.
//     The ring exposes only healthy instances, which is not what we want, and this
//     will lead to vast shard relocations and will deteriorate data locality if
//     instances leave and join the ring frequently.
//     Currently, the heartbeat timeout is set to 1m by default, which should prevent
//     us from severe problems, but it's still a problem.
//  3. Health checks are useless. It's better to sorry than to ask for permission:
//     client should mark failed/slow instances and not rely on the ring to do so.
//  4. Implement stream statistics (see seriesPlacement interface). This could be done
//     using Count-Min Sketch, or Count-Min-Log Sketch,  or HyperLogLog(+(+)).
//  5. Push API should be streaming: use of batching is not efficient.

type seriesDistributor struct {
	tenantServices  map[tenantServiceKey]*tenantServicePlacement
	seriesPlacement seriesPlacement
	distribution    *distribution
}

type seriesPlacement interface {
	tenantServiceSize(tenantServiceKey, []shard) int
	tenantServiceSeriesShard(*distributormodel.ProfileSeries, []shard) int
}

type defaultSeriesPlacement struct{}

func (defaultSeriesPlacement) tenantServiceSize(tenantServiceKey, []shard) int { return 2 }

func (defaultSeriesPlacement) tenantServiceSeriesShard(s *distributormodel.ProfileSeries, shards []shard) int {
	k := fnv64(phlaremodel.LabelPairsString(s.Labels))
	return int(k % uint64(len(shards)))
}

func newSeriesDistributor(r ring.ReadRing) (d *seriesDistributor, err error) {
	d = &seriesDistributor{seriesPlacement: defaultSeriesPlacement{}}
	if d.distribution, err = getDistribution(r, maxDistributionAge); err != nil {
		return nil, fmt.Errorf("series distributor: %w", err)
	}
	d.tenantServices = make(map[tenantServiceKey]*tenantServicePlacement)
	return d, nil
}

func (d *seriesDistributor) buildTrackers(tenantID string, series []*distributormodel.ProfileSeries) []*profileTracker {
	var size int
	for _, s := range series {
		d.append(tenantID, s)
		size++
	}
	trackers := make([]*profileTracker, 0, size)
	for _, p := range d.tenantServices {
		for _, s := range p.series {
			t := &profileTracker{profile: s}
			t.shard, t.instances = p.pickShard(s)
			trackers = append(trackers, t)
		}
	}
	// Do not retain the series, but do keep shards:
	// profileTracker references ring instances.
	d.tenantServices = nil
	return trackers
}

func (d *seriesDistributor) append(tenant string, s *distributormodel.ProfileSeries) {
	k := newTenantServiceKey(tenant, s.Labels)
	p, ok := d.tenantServices[k]
	if !ok {
		p = d.newTenantServicePlacement(k)
		d.tenantServices[k] = p
	}
	p.series = append(p.series, s)
}

// Although a request may contain multiple series
// that belong to different services, the tenant is
// always the same (as of now).
type tenantServiceKey struct {
	tenant  string
	service string
}

func newTenantServiceKey(tenant string, seriesLabels []*v1.LabelPair) tenantServiceKey {
	service := phlaremodel.Labels(seriesLabels).Get(phlaremodel.LabelNameServiceName)
	return tenantServiceKey{
		tenant:  tenant,
		service: service,
	}
}

func (k tenantServiceKey) hash() uint64 { return fnv64(k.tenant, k.service) }

const minShardsPerTenantService = 3

func (d *seriesDistributor) newTenantServicePlacement(key tenantServiceKey) *tenantServicePlacement {
	size := d.seriesPlacement.tenantServiceSize(key, d.distribution.shards)
	if size <= 0 {
		size = len(d.distribution.shards)
	}
	return &tenantServicePlacement{
		seriesDistributor: d,
		tenantServiceKey:  key,
		series:            make([]*distributormodel.ProfileSeries, 0, 16),
		// scope is a slice of shards that belong to the service.
		// It might be larger than the actual number of shards allowed for use.
		// In case of a delivery failure, at least minShardsPerTenantService
		// options of the shard placement (instances) are available: the series
		// will be sent to another shard location (ingester), but will still be
		// associated with the shard.
		scope: d.distribution.serviceShards(max(size, minShardsPerTenantService), key.hash()),
		size:  size,
	}
}

type tenantServicePlacement struct {
	*seriesDistributor
	tenantServiceKey
	series []*distributormodel.ProfileSeries
	scope  []shard
	size   int
}

// Pick the exact shard for the key from N options
// and find instances where the shard may be placed.
func (p *tenantServicePlacement) pickShard(s *distributormodel.ProfileSeries) (uint32, []*ring.InstanceDesc) {
	// Limit the scope for selection to the actual number
	// of shards, allowed for the tenant service.
	i := p.seriesPlacement.tenantServiceSeriesShard(s, p.scope[:p.size])
	x := p.scope[i]
	instances := make([]*ring.InstanceDesc, len(p.scope))
	for j, o := range p.scope {
		instances[j] = &p.distribution.desc[o.instance]
	}
	instances[0], instances[i] = instances[i], instances[0]
	return x.id, instances
}
