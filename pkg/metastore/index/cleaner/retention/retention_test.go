package retention

import (
	"iter"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/test"
)

type mockOverrides struct {
	defaultConfig Config
	overrides     map[string]Config
}

func (m *mockOverrides) Retention() (Config, iter.Seq2[string, Config]) {
	return m.defaultConfig, func(yield func(string, Config) bool) {
		for k, v := range m.overrides {
			if !yield(k, v) {
				return
			}
		}
	}
}

type testBlock struct {
	tenant    string
	shard     uint32
	createdAt time.Time
	minTime   time.Time
	maxTime   time.Time
}

func TestTimeBasedRetentionPolicy(t *testing.T) {
	type testCase struct {
		name               string
		defaultConfig      Config
		overrides          map[string]Config
		gracePeriod        time.Duration
		maxTombstones      int
		now                time.Time
		blocks             []testBlock
		expectedTombstones int
	}

	now := test.Time("2024-01-01T00:00:00Z")
	tests := []testCase{
		{
			name:          "no retention policies",
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-23 * time.Hour),
					minTime:   now.Add(-25 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
		},
		{
			name:          "no default retention policy but tenant override exists",
			defaultConfig: Config{},
			overrides: map[string]Config{
				"tenant-1": {RetentionPeriod: model.Duration(12 * time.Hour)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-23 * time.Hour),
					minTime:   now.Add(-25 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "tenant-1",
					shard:     2,
					createdAt: now.Add(-22 * time.Hour),
					minTime:   now.Add(-24 * time.Hour),
					maxTime:   now.Add(-18 * time.Hour),
				},
				{
					tenant:    "tenant-2",
					shard:     1,
					createdAt: now.Add(-25 * time.Hour),
					minTime:   now.Add(-26 * time.Hour),
					maxTime:   now.Add(-22 * time.Hour),
				},
			},
			expectedTombstones: 2,
		},
		{
			name:          "retention policy override shorter than partition",
			defaultConfig: Config{RetentionPeriod: model.Duration(24 * time.Hour)},
			overrides: map[string]Config{
				"tenant-2": {RetentionPeriod: model.Duration(6 * time.Hour)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-2",
					shard:     1,
					createdAt: now.Add(-9 * time.Hour),
					minTime:   now.Add(-9 * time.Hour),
					maxTime:   now.Add(-8 * time.Hour),
				},
			},
		},
		{
			name:          "retention policy override shorter than default",
			defaultConfig: Config{RetentionPeriod: model.Duration(24 * time.Hour)},
			overrides: map[string]Config{
				"tenant-2": {RetentionPeriod: model.Duration(4 * time.Hour)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     2,
					createdAt: now.Add(-18 * time.Hour),
					minTime:   now.Add(-18 * time.Hour),
					maxTime:   now.Add(-16 * time.Hour),
				},
				{
					tenant:    "tenant-2",
					shard:     1,
					createdAt: now.Add(-12 * time.Hour),
					minTime:   now.Add(-12 * time.Hour),
					maxTime:   now.Add(-10 * time.Hour),
				},
			},
			expectedTombstones: 1,
		},
		{
			name:          "retention policy override longer than default",
			defaultConfig: Config{RetentionPeriod: model.Duration(12 * time.Hour)},
			overrides: map[string]Config{
				"tenant-1": {RetentionPeriod: model.Duration(24 * time.Hour)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1", // Default.
					shard:     1,
					createdAt: now.Add(-16 * time.Hour),
					minTime:   now.Add(-16 * time.Hour),
					maxTime:   now.Add(-18 * time.Hour),
				},
			},
		},
		{
			name:          "anonymous tenant retained due to other tenant shards",
			defaultConfig: Config{RetentionPeriod: model.Duration(12 * time.Hour)},
			overrides: map[string]Config{
				"tenant-1": {RetentionPeriod: model.Duration(48 * time.Hour)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "tenant-2", // Default.
					shard:     1,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "",
					shard:     1,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
			expectedTombstones: 1,
		},
		{
			name:          "anonymous tenant deleted when no other tenant shards",
			defaultConfig: Config{RetentionPeriod: model.Duration(12 * time.Hour)},
			overrides:     map[string]Config{},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "",
					shard:     1,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
			expectedTombstones: 1,
		},
		{
			name:          "max tombstones limit reached",
			defaultConfig: Config{RetentionPeriod: model.Duration(12 * time.Hour)},
			overrides:     map[string]Config{},
			gracePeriod:   time.Hour,
			maxTombstones: 2,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "tenant-1",
					shard:     2,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "tenant-1",
					shard:     3,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
			expectedTombstones: 2,
		},
		{
			name:          "multiple tenant overrides with different retention periods",
			defaultConfig: Config{RetentionPeriod: model.Duration(24 * time.Hour)},
			overrides: map[string]Config{
				"tenant-short":    {RetentionPeriod: model.Duration(12 * time.Hour)},
				"tenant-infinite": {RetentionPeriod: 0},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-infinite",
					shard:     1,
					createdAt: now.Add(-180 * 24 * time.Hour),
					minTime:   now.Add(-180 * 24 * time.Hour),
					maxTime:   now.Add(-180*24*time.Hour + time.Hour),
				},
				{
					tenant:    "tenant-short",
					shard:     2,
					createdAt: now.Add(-30 * time.Hour),
					minTime:   now.Add(-30 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
				{
					tenant:    "default-tenant",
					shard:     3,
					createdAt: now.Add(-20 * time.Hour),
					minTime:   now.Add(-20 * time.Hour),
					maxTime:   now.Add(-18 * time.Hour),
				},
			},
			expectedTombstones: 1,
		},
		{
			name: "zero retention period means infinite retention",
			overrides: map[string]Config{
				"tenant-1": {RetentionPeriod: model.Duration(0)},
			},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-23 * time.Hour),
					minTime:   now.Add(-25 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
		},
		{
			name:          "zero retention period means infinite retention",
			defaultConfig: Config{RetentionPeriod: model.Duration(0)},
			gracePeriod:   time.Hour,
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "tenant-1",
					shard:     1,
					createdAt: now.Add(-23 * time.Hour),
					minTime:   now.Add(-25 * time.Hour),
					maxTime:   now.Add(-20 * time.Hour),
				},
			},
		},
		{
			name:          "partition exactly at retention boundary",
			defaultConfig: Config{RetentionPeriod: model.Duration(24 * time.Hour)},
			maxTombstones: 10,
			now:           now,
			blocks: []testBlock{
				{
					tenant:    "default-tenant",
					shard:     3,
					createdAt: now.Add(-26 * time.Hour),
					minTime:   now.Add(-26 * time.Hour),
					maxTime:   now.Add(-24 * time.Hour),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := test.BoltDB(t)
			store := indexstore.NewIndexStore()
			require.NoError(t, db.Update(store.CreateBuckets))
			defer db.Close()

			policy := NewTimeBasedRetentionPolicy(
				log.NewNopLogger(),
				&mockOverrides{
					defaultConfig: tc.defaultConfig,
					overrides:     tc.overrides,
				},
				tc.maxTombstones,
				tc.gracePeriod,
				tc.now,
			)

			const partitionDuration = 6 * time.Hour
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				for _, block := range tc.blocks {
					p := indexstore.NewPartition(block.createdAt.Truncate(partitionDuration), partitionDuration)
					s := indexstore.NewShard(p, block.tenant, block.shard)
					require.NoError(t, s.Store(tx, &metastorev1.BlockMeta{
						Id:          test.ULID(block.createdAt.Format(time.RFC3339)),
						Tenant:      1,
						Shard:       block.shard,
						MinTime:     block.minTime.UnixNano(),
						MaxTime:     block.maxTime.UnixNano(),
						StringTable: []string{"", block.tenant},
					}))
				}
				return nil
			}))

			require.NoError(t, db.View(func(tx *bbolt.Tx) error {
				// Multiple lines for better debugging.
				partitions := store.Partitions(tx)
				tombstones := policy.CreateTombstones(tx, partitions)
				assert.Equal(t, tc.expectedTombstones, len(tombstones))
				return nil
			}))
		})
	}
}
