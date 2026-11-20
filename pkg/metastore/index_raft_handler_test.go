package metastore

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index/tombstones"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestAddBlock_Idempotent(t *testing.T) {
	for _, tc := range []struct {
		name               string
		adds               int
		addAfterScheduling bool
	}{
		{"single_add", 1, false},
		{"duplicate_while_queued", 2, false},
		{"duplicate_after_scheduling", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := test.BoltDB(t)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			logger := log.NewNopLogger()
			idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
			ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
			cfg := compactor.DefaultConfig()
			cfg.Levels = []compactor.LevelConfig{{MaxBlocks: 1}}
			c := compactor.NewCompactor(cfg, compactor.NewStore(), ts, nil)
			handler := NewIndexCommandHandler(logger, idx, ts, c)
			require.NoError(t, db.Update(idx.Init))
			require.NoError(t, db.Update(ts.Init))
			require.NoError(t, db.Update(c.Init))

			now := test.Time("2024-09-11T07:00:00Z")
			block := &metastorev1.BlockMeta{
				Id:          test.ULID("2024-09-11T07:00:00Z"),
				Shard:       1,
				MinTime:     now.UnixMilli(),
				MaxTime:     now.UnixMilli() + 1,
				StringTable: []string{""},
			}
			addBlock := func(raftIndex uint64) {
				t.Helper()
				require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
					_, err := handler.AddBlock(t.Context(), tx,
						&raft.Log{Index: raftIndex, AppendedAt: now},
						&metastorev1.AddBlockRequest{Block: block.CloneVT()})
					return err
				}))
			}
			for i := range tc.adds {
				addBlock(uint64(i + 1))
			}

			cmd := &raft.Log{Index: uint64(tc.adds + 1), AppendedAt: now}
			job, err := c.NewPlan(cmd).CreateJob()
			require.NoError(t, err)
			require.NotNil(t, job)
			require.Equal(t, []string{block.Id}, job.SourceBlocks)
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				return c.UpdatePlan(tx, &raft_log.CompactionPlanUpdate{
					NewJobs: []*raft_log.NewCompactionJob{{Plan: job}},
				})
			}))
			if tc.addAfterScheduling {
				addBlock(cmd.Index + 1)
			}
			job, err = c.NewPlan(cmd).CreateJob()
			require.NoError(t, err)
			require.Nil(t, job, "scheduled blocks must stay out of the in-memory queue")

			c = compactor.NewCompactor(cfg, compactor.NewStore(), ts, nil)
			require.NoError(t, db.View(c.Restore))
			job, err = c.NewPlan(cmd).CreateJob()
			require.NoError(t, err)
			require.Nil(t, job, "restore must not requeue an already scheduled block")
		})
	}
}
