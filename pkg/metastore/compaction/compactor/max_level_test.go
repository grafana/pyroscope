package compactor

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockcompactor"
)

func TestConfig_MaxCompactionLevel(t *testing.T) {
	var cfg Config
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlagsWithPrefix("metastore.", flags)
	require.Equal(t, uint(3), cfg.MaxCompactionLevel)
	require.True(t, cfg.acceptsLevel(2))
	require.False(t, cfg.acceptsLevel(3))
	for _, limit := range []uint{3, 4, 5, 8, 100} {
		require.NoError(t, flags.Parse([]string{fmt.Sprintf("-metastore.max-compaction-level=%d", limit)}))
		require.NoError(t, cfg.Validate())
		require.True(t, cfg.acceptsLevel(uint32(limit-1)))
		require.False(t, cfg.acceptsLevel(uint32(limit)))
		require.Equal(t, uint(10), cfg.maxBlocks(uint32(limit-1)))
	}
	for _, limit := range []uint{0, 1, 2} {
		cfg.MaxCompactionLevel = limit
		require.Error(t, cfg.Validate())
	}
}

func TestCompactor_MaxLevel_RestoreAndLower(t *testing.T) {
	for _, limit := range []uint{4, 5, 8} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			testCompactorRestoreAndLower(t, limit)
		})
	}
}

func testCompactorRestoreAndLower(t *testing.T, limit uint) {
	db := test.BoltDB(t)
	cfg := DefaultConfig()
	tombstones := new(mockcompactor.MockTombstones)
	tombstones.On("ListTombstones", mock.Anything).
		Return(iter.NewEmptyIterator[*metastorev1.Tombstones]())
	c := NewCompactor(cfg, NewStore(), tombstones, nil)
	require.NoError(t, db.Update(c.Init))
	now := test.Time("2024-09-01T00:00:00Z")
	entry := compaction.BlockEntry{Index: 1, AppendedAt: now.UnixNano(), ID: test.ULID(now.Format(time.RFC3339)), Tenant: "A", Level: uint32(limit - 1)}

	cfg.MaxCompactionLevel = limit
	c = NewCompactor(cfg, NewStore(), tombstones, nil)
	for _, tenant := range []string{"A", "B"} {
		for n := range 10 {
			entry.Index++
			entry.Tenant = tenant
			entry.ID = test.ULID(now.Add(time.Duration(entry.Index) * time.Hour).Format(time.RFC3339))
			entry.AppendedAt = now.Add(time.Duration(n) * time.Hour).UnixNano()
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error { return c.Compact(tx, entry) }))
		}
	}
	require.Len(t, c.queue.levels, int(limit))

	p := c.NewPlan(&raft.Log{AppendedAt: now.Add(48 * time.Hour)})
	var jobs []*raft_log.NewCompactionJob
	for _, tenant := range []string{"A", "B"} {
		job, err := p.CreateJob()
		require.NoError(t, err)
		require.NotNil(t, job)
		require.Equal(t, uint32(limit-1), job.CompactionLevel)
		require.Equal(t, tenant, job.Tenant)
		require.Len(t, job.SourceBlocks, 10)
		jobs = append(jobs, &raft_log.NewCompactionJob{Plan: job})
	}
	job, err := p.CreateJob()
	require.NoError(t, err)
	require.Nil(t, job)

	// Lowering must retain persisted candidates, stop planning higher jobs,
	// and still apply an already committed leader plan without resurrection.
	cfg.MaxCompactionLevel = 3
	c = NewCompactor(cfg, NewStore(), tombstones, nil)
	require.NoError(t, db.View(c.Restore))
	job, err = c.NewPlan(&raft.Log{AppendedAt: now.Add(48 * time.Hour)}).CreateJob()
	require.NoError(t, err)
	require.Nil(t, job)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return c.UpdatePlan(tx, &raft_log.CompactionPlanUpdate{NewJobs: jobs})
	}))
	cfg.MaxCompactionLevel = limit
	c = NewCompactor(cfg, NewStore(), tombstones, nil)
	require.NoError(t, db.View(c.Restore))
	job, err = c.NewPlan(&raft.Log{AppendedAt: now.Add(48 * time.Hour)}).CreateJob()
	require.NoError(t, err)
	require.Nil(t, job)
}

func TestPlan_MaxLevel_TimeRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCompactionLevel = 8
	for level := uint32(0); level < 8; level++ {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			p := &jobPlan{config: &cfg}
			p.reset(compactionKey{level: level})
			start := test.Time("2024-09-01T00:00:00Z")
			require.True(t, p.tryAdd(test.ULID(start.Format(time.RFC3339))))
			span := 3 * time.Hour
			if level >= 3 {
				span = 24 * time.Hour
			}
			require.True(t, p.tryAdd(test.ULID(start.Add(span).Format(time.RFC3339))))
			require.False(t, p.tryAdd(test.ULID(start.Add(span+time.Second).Format(time.RFC3339))))
		})
	}
}
