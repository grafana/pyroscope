package metastore

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
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

	m := newMetastoreState(util.Logger, db, reg, &config.Compaction, &index.DefaultConfig)
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
}
