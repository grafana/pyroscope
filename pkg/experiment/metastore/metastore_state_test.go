package metastore

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestMetadataStateManagement(t *testing.T) {
	reg := prometheus.DefaultRegisterer
	config := Config{
		DataDir:    t.TempDir(),
		Compaction: CompactionConfig{},
	}
	db := newDB(config, util.Logger, newMetastoreMetrics(reg))
	err := db.open(false)
	require.NoError(t, err)

	m := newMetastoreState(util.Logger, db, reg, &config.Compaction)
	require.NotNil(t, m)

	t.Run("restore compaction state", func(t *testing.T) {
		// populate state with block queues and jobs
		for i := 0; i < 420; i++ {
			err = db.boltdb.Update(func(tx *bbolt.Tx) error {
				block := &metastorev1.BlockMeta{
					Id:    fmt.Sprintf("b-%d", i),
					Shard: uint32(i % 4),
				}
				err := m.compactBlock(block, tx, uint64(i))
				require.NoError(t, err)
				return nil
			})
		}

		// clear state
		m.compactionJobQueue = newJobQueue(m.compactionConfig.JobLeaseDuration.Nanoseconds())
		m.compactionJobBlockQueues = make(map[tenantShard]*compactionJobBlockQueue)

		// restore state from db
		err = db.boltdb.Update(func(tx *bbolt.Tx) error {
			return m.restoreCompactionPlan(tx)
		})
		require.NoError(t, err)

		require.Equal(t, 20, len(m.compactionJobQueue.jobs))
		require.Equal(t, 4, len(m.compactionJobBlockQueues))
		queue := m.getOrCreateCompactionBlockQueue(tenantShard{
			tenant: "",
			shard:  3,
		})
		require.Equal(t, 1, len(queue.blocksByLevel))
		require.Equal(t, 5, len(queue.blocksByLevel[0]))
	})

	t.Run("restore block state", func(t *testing.T) {
		for i := 0; i < 420; i++ {
			err = db.boltdb.Update(func(tx *bbolt.Tx) error {
				block := &metastorev1.BlockMeta{
					Id:    ulid.MustNew(ulid.Now(), rand.Reader).String(),
					Shard: uint32(i % 4),
				}
				key := []byte(block.Id)
				value, err := block.MarshalVT()
				require.NoError(t, err)
				partKey := m.index.getPartitionKey(block.Id)
				err = updateBlockMetadataBucket(tx, partKey, block.Shard, block.TenantId, func(bucket *bbolt.Bucket) error {
					return bucket.Put(key, value)
				})
				return err
			})
			require.NoError(t, err)
		}

		// restore from db
		m.index.loadPartitions()
		require.NoError(t, err)

		require.Equal(t, 1, len(m.index.partitions))
		for _, p := range m.index.partitionMap {
			require.Equal(t, 4, len(p.shards))
			for _, s := range p.shards {
				require.Equal(t, 1, len(s.tenants))
				for _, ten := range s.tenants {
					require.Equal(t, 105, len(ten.blocks))
				}
			}
		}
	})
}
