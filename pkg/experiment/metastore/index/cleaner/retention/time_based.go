package retention

import (
	"flag"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
)

type Config struct {
	RetentionPeriod time.Duration `yaml:"retention_period"`
}

type Overrides interface {
	RetentionOverrides(tenantID string) Config
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.RetentionPeriod, prefix+"retention-period", 0, "Retention period for the data. 0 means data never deleted.")
}

type TimeBasedRetentionPolicy struct{}

func (p *TimeBasedRetentionPolicy) View(*indexstore.Partition) bool { return true }

func (p *TimeBasedRetentionPolicy) Tombstones() []*metastorev1.Tombstones {
	// 	dst = dst[:0]
	// 	for tenant, shards := range p.TenantShards {
	// 		for shard := range shards {
	// 			dst = append(dst, Shard{
	// 				Partition: p.Key,
	// 				Tenant:    tenant,
	// 				Shard:     shard,
	// 			})
	// 		}
	// 	}
	//
	// 	slices.SortFunc(dst, func(a, b Shard) int {
	// 		t := cmp.Compare(a.Tenant, b.Tenant)
	// 		if t != 0 {
	// 			return cmp.Compare(a.Shard, b.Shard)
	// 		}
	// 		return t
	// 	})
	//
	//	&metastorev1.Tombstones{
	//			Partition: &metastorev1.PartitionTombstone{
	//				Name:      shard.TombstoneName(),
	//				Timestamp: shard.Partition.Timestamp.UnixNano(),
	//				Shard:     shard.Shard,
	//				Tenant:    shard.Tenant,
	//			},
	//		})
	//	}
	//
	return nil
}
