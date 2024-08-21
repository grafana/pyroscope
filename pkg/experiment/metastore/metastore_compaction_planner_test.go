package metastore

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

func Test_MaintainSeparateBlockQueues(t *testing.T) {
	m := initState(t)
	_ = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		_ = m.compactBlock(createBlock(1, 0, "", 0), tx, 0)
		_ = m.compactBlock(createBlock(2, 0, "", 0), tx, 0)
		_ = m.compactBlock(createBlock(3, 0, "", 0), tx, 0)
		_ = m.compactBlock(createBlock(4, 1, "", 0), tx, 0)
		_ = m.compactBlock(createBlock(5, 1, "", 0), tx, 0)
		_ = m.compactBlock(createBlock(6, 1, "tenant1", 1), tx, 0)
		_ = m.compactBlock(createBlock(7, 1, "tenant2", 1), tx, 0)
		_ = m.compactBlock(createBlock(8, 1, "tenant1", 1), tx, 0)
		return nil
	})
	require.Equal(t, 3, getQueueLen(m, 0, "", 0))
	require.Equal(t, 2, getQueueLen(m, 1, "", 0))
	require.Equal(t, 2, getQueueLen(m, 1, "tenant1", 1))
	require.Equal(t, 1, getQueueLen(m, 1, "tenant2", 1))
	verifyCompactionState(t, m)
}

func Test_CreateJobs(t *testing.T) {
	m := initState(t)
	_ = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		for i := 0; i < 420; i++ {
			_ = m.compactBlock(createBlock(i, i%4, "", 0), tx, 0)
		}
		return nil
	})
	require.Equal(t, 5, getQueueLen(m, 0, "", 0))
	require.Equal(t, 5, getQueueLen(m, 1, "", 0))
	require.Equal(t, 5, getQueueLen(m, 2, "", 0))
	require.Equal(t, 5, getQueueLen(m, 3, "", 0))
	require.Equal(t, 20, len(m.compactionJobQueue.jobs))
	verifyCompactionState(t, m)
}

func initState(t *testing.T) *metastoreState {
	reg := prometheus.DefaultRegisterer
	config := Config{
		DataDir: t.TempDir(),
	}
	db := newDB(config, util.Logger, newMetastoreMetrics(reg))
	err := db.open(false)
	require.NoError(t, err)

	m := newMetastoreState(util.Logger, db, reg)
	require.NotNil(t, m)
	return m
}

func createBlock(id int, shard int, tenant string, level int) *metastorev1.BlockMeta {
	return &metastorev1.BlockMeta{
		Id:              fmt.Sprintf("b-%d", id),
		Shard:           uint32(shard),
		TenantId:        tenant,
		CompactionLevel: uint32(level),
	}
}

func getQueueLen(m *metastoreState, shard int, tenant string, level int) int {
	return len(m.getOrCreateCompactionBlockQueue(tenantShard{
		tenant: tenant,
		shard:  uint32(shard),
	}).blocksByLevel[uint32(level)])
}

func verifyCompactionState(t *testing.T, m *metastoreState) {
	stateFromDb := newMetastoreState(util.Logger, m.db, prometheus.DefaultRegisterer)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		return stateFromDb.restoreCompactionPlan(tx)
	})
	require.NoError(t, err)

	require.Equalf(t, len(m.compactionJobQueue.jobs), len(stateFromDb.compactionJobQueue.jobs), "job queues different")
	for name, _ := range m.compactionJobQueue.jobs {
		require.Truef(t, stateFromDb.compactionJobQueue.jobs[name] != nil, "missing compaction job %s", name)
	}
	require.Equalf(t, len(m.compactionJobBlockQueues), len(stateFromDb.compactionJobBlockQueues), "block queues different")
	for key := range m.compactionJobBlockQueues {
		require.Truef(t, stateFromDb.compactionJobBlockQueues[key] != nil, "no queue for key %v", key)
		for level, blocks := range m.compactionJobBlockQueues[key].blocksByLevel {
			require.Truef(t, stateFromDb.compactionJobBlockQueues[key].blocksByLevel[level] != nil, "no queue for level %d", level)
			require.Equalf(t, blocks, stateFromDb.compactionJobBlockQueues[key].blocksByLevel[level], "queues different for level %d", level)
		}
	}
}
