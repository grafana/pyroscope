package metastore

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/compactor"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction/scheduler"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsmversion"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index"
	"github.com/grafana/pyroscope/v2/pkg/metastore/index/tombstones"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestCompactionPlan_MaxLevelAdmissionIsReplicated(t *testing.T) {
	for _, version := range []fsmversion.Version{fsmversion.Unversioned, fsmversion.Baseline, fsmversion.ConfigurableCompactionLevels} {
		t.Run(fmt.Sprintf("version=%d", version), func(t *testing.T) {
			testCompactionPlanMaxLevelAdmission(t, version)
		})
	}
}

func testCompactionPlanMaxLevelAdmission(t *testing.T, version fsmversion.Version) {
	for _, localMax := range []uint32{3, 4, 5, 8} {
		for _, replicatedMax := range []uint32{0, 3, 4, 5, 8} {
			t.Run(fmt.Sprintf("local=%v/replicated=%v", localMax, replicatedMax), func(t *testing.T) {
				db := test.BoltDB(t)
				cfg := compactor.DefaultConfig()
				cfg.MaxCompactionLevel = uint(localMax)
				logger := log.NewNopLogger()
				idx := index.NewIndex(logger, index.NewStore(), index.DefaultConfig, nil)
				ts := tombstones.NewTombstones(tombstones.NewStore(), nil)
				store := compactor.NewStore()
				c := compactor.NewCompactor(cfg, store, ts, nil)
				sc := scheduler.NewScheduler(scheduler.Config{}, scheduler.NewStore(), nil)
				for _, init := range []func(*bbolt.Tx) error{idx.Init, ts.Init, c.Init, sc.Init} {
					require.NoError(t, db.Update(init))
				}
				state := fsmversion.NewState(logger, fsmversion.Latest, nil)
				require.NoError(t, db.Update(state.Init))
				require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
					_, err := state.SetVersion(t.Context(), tx, &raft.Log{Term: 1}, &raft_log.SetFSMVersionRequest{Term: 1, Version: uint32(version)})
					return err
				}))
				h := NewCompactionCommandHandler(logger, idx, c, c, sc, ts, localMax)
				cmd := &raft.Log{Term: 1, Index: 1, AppendedAt: test.Time("2024-09-01T00:00:00Z")}
				require.NoError(t, db.View(func(tx *bbolt.Tx) error {
					prepared, err := h.GetCompactionPlanUpdate(t.Context(), tx, cmd, &raft_log.GetCompactionPlanUpdateRequest{})
					require.NoError(t, err)
					want := uint32(0)
					if version >= fsmversion.ConfigurableCompactionLevels {
						want = localMax
					}
					require.Equal(t, want, prepared.PlanUpdate.MaxCompactionLevel)
					return nil
				}))
				update := &raft_log.CompactionPlanUpdate{MaxCompactionLevel: replicatedMax}
				for _, level := range []uint32{2, 3, 4, 5, 6, 7, 8} {
					id := test.ULID(cmd.AppendedAt.Add(time.Duration(level) * time.Minute).Format(time.RFC3339))
					update.CompletedJobs = append(update.CompletedJobs, &raft_log.CompletedCompactionJob{
						State: &raft_log.CompactionJobState{Name: fmt.Sprint(level), CompactionLevel: level - 1},
						CompactedBlocks: &metastorev1.CompactedBlocks{
							SourceBlocks: &metastorev1.BlockList{Tenant: "A"},
							NewBlocks:    []*metastorev1.BlockMeta{{Id: id, Tenant: 1, StringTable: []string{"", "A"}, CompactionLevel: level}},
						},
					})
				}
				// Round trip through the actual log encoding. Zero is omitted,
				// matching entries produced by binaries predating this feature.
				encoded, err := update.MarshalVT()
				require.NoError(t, err)
				var decoded raft_log.CompactionPlanUpdate
				require.NoError(t, decoded.UnmarshalVT(encoded))
				require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
					_, err := h.UpdateCompactionPlan(t.Context(), tx, cmd, &raft_log.UpdateCompactionPlanRequest{Term: 1, PlanUpdate: &decoded})
					return err
				}))
				require.NoError(t, db.View(func(tx *bbolt.Tx) error {
					entries := store.ListEntries(tx)
					defer entries.Close()
					var levels []uint32
					for entries.Next() {
						levels = append(levels, entries.At().Level)
					}
					require.NoError(t, entries.Err())
					var want []uint32
					limit := uint32(3)
					if version >= fsmversion.ConfigurableCompactionLevels {
						limit = max(3, replicatedMax)
					}
					for level := uint32(2); level < limit; level++ {
						want = append(want, level)
					}
					require.Equal(t, want, levels)
					return nil
				}))
			})
		}
	}
}
