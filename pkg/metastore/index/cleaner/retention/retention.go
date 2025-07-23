package retention

import (
	"flag"
	"iter"
	"math"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
)

// Policy determines which parts of the index should be retained or deleted.
type Policy interface {
	// CreateTombstones examines the provided partitions and returns tombstones
	// for shards that should be deleted according to the policy.
	CreateTombstones(*bbolt.Tx, iter.Seq[indexstore.Partition]) []*metastorev1.Tombstones
}

type Config struct {
	RetentionPeriod model.Duration `yaml:"retention_period" doc:"hidden"`
}

type Overrides interface {
	Retention() (defaults Config, overrides iter.Seq2[string, Config])
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	c.RetentionPeriod = model.Duration(time.Hour * 24 * 31)
	f.Var(&c.RetentionPeriod, "retention-period", "Retention period for the data. 0 means data never deleted.")
}

// TimeBasedRetentionPolicy implements a retention policy based on time.
type TimeBasedRetentionPolicy struct {
	logger        log.Logger
	overrides     map[string]*marker
	gracePeriod   time.Duration
	maxTombstones int

	markers       []*marker
	defaultPeriod *marker
	tombstones    []*metastorev1.Tombstones
}

type marker struct {
	tenantID  string
	timestamp time.Time
}

func NewTimeBasedRetentionPolicy(
	logger log.Logger,
	overrides Overrides,
	maxTombstones int,
	gracePeriod time.Duration,
	now time.Time,
) *TimeBasedRetentionPolicy {
	defaults, tenantOverrides := overrides.Retention()
	rp := TimeBasedRetentionPolicy{
		logger:        logger,
		overrides:     make(map[string]*marker),
		tombstones:    make([]*metastorev1.Tombstones, 0, maxTombstones),
		maxTombstones: maxTombstones,
		gracePeriod:   gracePeriod,
	}
	// Markers indicate the time before which data should be deleted
	// for a given tenant.
	for tenantID, override := range tenantOverrides {
		if defaults.RetentionPeriod == override.RetentionPeriod {
			continue
		}
		// An override is defined for the tenant, so we need to adjust the
		// retention period for it. By default, we assume that the retention
		// period is not defined, i.e. is infinite.
		var timestamp time.Time // zero value means no retention period.
		if period := time.Duration(override.RetentionPeriod); period > 0 {
			timestamp = now.Add(-period)
		}
		m := &marker{
			tenantID:  tenantID,
			timestamp: timestamp,
		}
		rp.markers = append(rp.markers, m)
		rp.overrides[tenantID] = m
	}
	// The default retention period is handled separately: we won't create
	// the marker if the retention period is not set. This allows us to avoid
	// checking all partition tenant shards, and instead only check specific
	// tenants that have a defined retention policy.
	if defaults.RetentionPeriod > 0 {
		rp.defaultPeriod = &marker{timestamp: now.Add(-time.Duration(defaults.RetentionPeriod))}
		rp.markers = append(rp.markers, rp.defaultPeriod)
	}
	// It is fine if there are marker pointing to the same time: for example,
	// if an override is set explicitly for a tenant, but it matches the
	// default value.
	slices.SortFunc(rp.markers, func(a, b *marker) int {
		return a.timestamp.Compare(b.timestamp)
	})

	return &rp
}

func (rp *TimeBasedRetentionPolicy) CreateTombstones(tx *bbolt.Tx, partitions iter.Seq[indexstore.Partition]) []*metastorev1.Tombstones {
	if len(rp.markers) == 0 {
		level.Debug(rp.logger).Log("msg", "no retention policies defined, skipping")
		return nil
	}
	for _, m := range rp.markers {
		level.Debug(rp.logger).Log("msg", "found retention marker", "tenant", m.tenantID, "timestamp", m.timestamp)
	}
	rp.tombstones = rp.tombstones[:0]
	for p := range partitions {
		if len(rp.tombstones) >= rp.maxTombstones {
			break
		}
		if !rp.processPartition(tx, p) {
			break
		}
	}
	return rp.tombstones
}

func (rp *TimeBasedRetentionPolicy) processPartition(tx *bbolt.Tx, p indexstore.Partition) bool {
	// We want to find the markers that are before the partition end, i.e. the
	// markers that indicate the time before which data should be deleted. For
	// tenants D and E we need to inspect the partition. Otherwise, if there
	// are no markers after the partition end, we stop.
	//
	//            | partition            |
	//            | start            end |             t
	//  ----------|----------------------x------------->
	//            *         *            *  *      *
	//  markers:  A         B            C  D      E
	//
	// Note that we also add a grace period to the partition end time, so that
	// we won't be checking it for this period. Since tombstones are only
	// created after the shard max time is before the marker timestamp, and the
	// distance between them might be large (hours), we would be wasting time
	// if were inspecting the partition right away.
	partitionEnd := &marker{timestamp: p.EndTime().Add(rp.gracePeriod)}
	logger := log.With(rp.logger, "partition", p.String())
	level.Debug(logger).Log(
		"msg", "processing partition",
		"partition_end_marker", partitionEnd.timestamp,
		"retention_markers", len(rp.markers),
	)

	i, _ := slices.BinarySearchFunc(rp.markers, partitionEnd, func(a, b *marker) int {
		return a.timestamp.Compare(b.timestamp)
	})
	if i >= len(rp.markers) {
		// All markers are before the partition end: it can't be deleted.
		// We can stop here: no partitions after this one will have deletion
		// markers that are before the partition end.
		level.Debug(logger).Log("msg", "partition has not passed the retention period, skipping")
		return false
	}

	q := p.Query(tx)
	if q == nil {
		level.Warn(logger).Log("msg", "cannot find partition, skipping")
		return true
	}

	// The anonymous tenant is ignored here, we only collect tombstones for the
	// specific tenants, which have a defined retention policy.
	if rp.defaultPeriod == nil || rp.defaultPeriod.timestamp.Before(partitionEnd.timestamp) {
		// Fast path for the case when there are markers very far in the future
		// relatively the default marker, or no default marker at all.
		//
		// The default retention period has not expired yet, so we don't need
		// to inspect all the tenants. Instead, we can just examine the markers
		// that are after the partition end: these tenants have retention
		// period shorter than the default one. This is useful in case if the
		// tenant data is deleted by setting very short retention period: we
		// won't check each and every partition tenant shard.
		level.Debug(logger).Log("msg", "creating tombstones for tenant markers", "retention_markers", len(rp.markers[i:]))
		rp.createTombstonesForMarkers(q, rp.markers[i:])
	} else {
		// Otherwise, we need to inspect all the tenants in the partition.
		// There's no point in checking the markers: either most of them
		// will result in tombstones, or have already been deleted (e.g.,
		// there's one tenant with an infinite retention period).
		level.Debug(logger).Log("msg", "creating tombstones for partition tenants")
		rp.createTombstonesForTenants(q, partitionEnd)
	}

	// Finally, we need to check if the anonymous tenant has any tombstones to
	// collect. We only delete it if there are no other tenant shards in the
	// partition: this guarantees that we don't delete data that is still
	// needed, as we'd have the named tenant shards otherwise. Note that the
	// tombstones we created this far are not resulted in the deletion of
	// shards yet, so we will only delete the anonymous tenant on a second
	// pass.
	//
	// NOTE(kolesnikovae):
	//
	// The approach may result in keeping the anonymous tenant data longer than
	// necessary, but it should not be a problem in practice, as we assume that
	// it contains no blocks: those are removed at L0 compaction. However, the
	// shard-level structures such as tenant-shard buckets and string tables
	// are not removed until the shard is deleted, which may also affect the
	// index size. Ideally, we should seal partitions (create checkpoints) at
	// some point that would protect it from modifications; then, we could
	// delete the anon tenant shards safely, if there's no uncompacted data.
	//
	// An alternative approach would be to mark anon shards as we remove blocks
	// from them, or create tombstones. We cannot do this as a side effect, to
	// avoid state drift between the replicas (although this is arguable – such
	// deletion is a side effect per se, and it only concerns the local state),
	// but we can find the marks during the cleanup job. That could be
	// implemented as a separate retention policy.
	rp.createTombstonesForAnonTenant(q)
	return len(rp.tombstones) < rp.maxTombstones
}

func (rp *TimeBasedRetentionPolicy) createTombstonesForMarkers(q *indexstore.PartitionQuery, markers []*marker) {
	for _, m := range markers {
		if m.tenantID == "" {
			continue
		}
		if !rp.createTombstones(q, m) {
			return
		}
	}
}

func (rp *TimeBasedRetentionPolicy) createTombstonesForTenants(q *indexstore.PartitionQuery, partitionEnd *marker) {
	for tenantID := range q.Tenants() {
		if tenantID == "" {
			continue
		}
		var m *marker
		if o, ok := rp.overrides[tenantID]; ok {
			m = o
		} else if rp.defaultPeriod != nil {
			// Tenant-specific marker using the default retention period.
			m = &marker{tenantID: tenantID, timestamp: rp.defaultPeriod.timestamp}
		} else {
			// No retention policy for this tenant, and no default:
			// we retain the data indefinitely.
			continue
		}
		if m.timestamp.After(partitionEnd.timestamp) {
			if !rp.createTombstones(q, m) {
				return
			}
		}
	}
}

func (rp *TimeBasedRetentionPolicy) createTombstonesForAnonTenant(q *indexstore.PartitionQuery) {
	if rp.hasTenants(q) {
		// We have at least one tenant other than the anonymous one.
		// We cannot delete the anonymous tenant shard yet – continue.
		return
	}
	// Once shard max time passes the partition end time, we can
	// create tombstones for the anonymous tenant shard.
	level.Debug(rp.logger).Log("msg", "creating tombstones for anonymous tenant")
	// We want to bypass the timestamp check for the anonymous tenant:
	// we know that if all the other tenants have been processed, it's
	// safe to create tombstones for the anonymous tenant.
	rp.createTombstones(q, &marker{timestamp: time.UnixMilli(math.MaxInt64)})
}

func (rp *TimeBasedRetentionPolicy) hasTenants(q *indexstore.PartitionQuery) bool {
	var n int
	for tenant := range q.Tenants() {
		n++
		if n > 1 || tenant != "" {
			return true
		}
	}
	return false
}

func (rp *TimeBasedRetentionPolicy) createTombstones(q *indexstore.PartitionQuery, m *marker) bool {
	for shard := range q.Shards(m.tenantID) {
		if len(rp.tombstones) >= rp.maxTombstones {
			return false
		}
		maxTime := time.Unix(0, shard.ShardIndex.MaxTime)
		if maxTime.Before(m.timestamp) {
			// The shard does not contain data before the marker.
			name := shard.TombstoneName()
			level.Debug(rp.logger).Log("msg", "creating tombstone", "name", name)
			rp.tombstones = append(rp.tombstones, &metastorev1.Tombstones{
				Shard: &metastorev1.ShardTombstone{
					Name:      name,
					Timestamp: q.Timestamp.UnixNano(),
					Duration:  q.Duration.Nanoseconds(),
					Shard:     shard.Shard,
					Tenant:    shard.Tenant,
				},
			})
		}
	}
	return true
}
