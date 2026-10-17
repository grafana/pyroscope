package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

func testJobPlan(sourceBlocks, tombstones int) *raft_log.CompactionJobPlan {
	blocks := func(n int) []string {
		if n == 0 {
			return nil
		}
		s := make([]string, n)
		for i := range s {
			s[i] = fmt.Sprintf("01M14VWWJJEDW4TPDEFH%06d", i)
		}
		return s
	}
	plan := &raft_log.CompactionJobPlan{
		Name:            "8bc44b23a733bee2-T-tenant-a-S7-L1",
		Tenant:          "tenant-a",
		Shard:           7,
		CompactionLevel: 1,
		SourceBlocks:    blocks(sourceBlocks),
	}
	if tombstones > 0 {
		plan.Tombstones = []*metastorev1.Tombstones{{
			Blocks: &metastorev1.BlockTombstones{
				Name:            "tombstone",
				Tenant:          "tenant-a",
				Shard:           7,
				CompactionLevel: 1,
				Blocks:          blocks(tombstones),
			},
		}}
	}
	return plan
}

// The summary is parsed from the wire format by hand, so it must agree with
// the generated message. This is the guard against a schema change silently
// making the parser read the wrong fields.
func TestJobPlanSummary_matchesGeneratedMessage(t *testing.T) {
	for _, test := range []struct {
		sourceBlocks, tombstones int
	}{
		{0, 0},
		{1, 0},
		{20, 0},
		{20, 50},
		{0, 50},
	} {
		t.Run(fmt.Sprintf("blocks=%d/tombstones=%d", test.sourceBlocks, test.tombstones), func(t *testing.T) {
			plan := testJobPlan(test.sourceBlocks, test.tombstones)
			b, err := plan.MarshalVT()
			require.NoError(t, err)

			var summary JobPlanSummary
			require.NoError(t, unmarshalJobPlanSummary(&summary, b, true))
			assert.Equal(t, plan.Tenant, summary.Tenant)
			assert.Equal(t, plan.Shard, summary.Shard)
			assert.Equal(t, plan.CompactionLevel, summary.CompactionLevel)
			assert.Equal(t, uint32(len(plan.SourceBlocks)), summary.SourceBlocks)
			assert.Equal(t, plan.SourceBlocks, summary.SourceBlockIDs)

			// Without the identifiers the count must still be right, and
			// nothing should be retained.
			var counted JobPlanSummary
			require.NoError(t, unmarshalJobPlanSummary(&counted, b, false))
			assert.Equal(t, uint32(len(plan.SourceBlocks)), counted.SourceBlocks)
			assert.Nil(t, counted.SourceBlockIDs)
			assert.Equal(t, plan.Tenant, counted.Tenant)
		})
	}
}

// An empty tenant is what a level 0 plan carries, and proto3 does not encode
// it at all: the summary must report it as empty rather than fail.
func TestJobPlanSummary_emptyTenant(t *testing.T) {
	plan := &raft_log.CompactionJobPlan{Name: "job", Shard: 3, SourceBlocks: []string{"b1"}}
	b, err := plan.MarshalVT()
	require.NoError(t, err)

	var summary JobPlanSummary
	require.NoError(t, unmarshalJobPlanSummary(&summary, b, true))
	assert.Empty(t, summary.Tenant)
	assert.Equal(t, uint32(3), summary.Shard)
	assert.Equal(t, uint32(0), summary.CompactionLevel)
	assert.Equal(t, []string{"b1"}, summary.SourceBlockIDs)
}

func TestJobPlanSummary_invalid(t *testing.T) {
	var summary JobPlanSummary
	// A truncated varint cannot be parsed.
	assert.Error(t, unmarshalJobPlanSummary(&summary, []byte{0x18}, false))
}
