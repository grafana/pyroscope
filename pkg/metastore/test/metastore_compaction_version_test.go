package test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsmversion"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

// Preconfigure the higher maximum for the whole rollout. The replicated gate,
// rather than a second configuration rollout, activates higher-level admission.
func TestCompactionLevels_RollingActivationAndRestore(t *testing.T) {
	cfg := fsmVersionTestConfig()
	cfg.Compactor.MaxCompactionLevel = 5
	cfg.Compactor.Levels[0].MaxBlocks = 1 << 20
	cfg.Compactor.Levels[0].MaxAge = 0
	cfg.Compactor.Levels[2].MaxBlocks = 1
	cfg.Compactor.Levels[3].MaxBlocks = 1
	frequentSnapshots(cfg)
	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket(), supporting(1, 1, 1))
	t.Cleanup(ms.Close)
	requireActiveVersion(t, &ms, fsmversion.Baseline)

	addSource := func(tenant string) *metastorev1.BlockMeta {
		now := time.Now()
		md := &metastorev1.BlockMeta{
			Id:     ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(),
			Tenant: 1, Shard: 1, CompactionLevel: 2,
			MinTime: now.UnixMilli(), MaxTime: now.Add(time.Minute).UnixMilli(),
			StringTable: []string{"", tenant, "dataset", "service_name", "service"},
			Datasets: []*metastorev1.Dataset{{Tenant: 1, Name: 2,
				MinTime: now.UnixMilli(), MaxTime: now.Add(time.Minute).UnixMilli(), Labels: []int32{1, 3, 4}}},
		}
		_, err := ms.Client.AddBlock(t.Context(), &metastorev1.AddBlockRequest{Block: md})
		require.NoError(t, err)
		return md
	}
	complete := func(source *metastorev1.BlockMeta) *metastorev1.BlockMeta {
		var response *metastorev1.PollCompactionJobsResponse
		require.Eventually(t, func() bool {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var err error
			response, err = ms.Client.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
			return err == nil && len(response.CompactionJobs) == 1
		}, 10*time.Second, 20*time.Millisecond)
		job := response.CompactionJobs[0]
		require.Equal(t, source.CompactionLevel, job.CompactionLevel)
		require.Equal(t, []string{source.Id}, job.SourceBlocks)
		var token uint64
		for _, assignment := range response.Assignments {
			if assignment.Name == job.Name {
				token = assignment.Token
			}
		}
		require.NotZero(t, token)
		result := source.CloneVT()
		result.CompactionLevel++
		result.Id = ulid.MustNew(ulid.MustParse(source.Id).Time(), rand.Reader).String()
		_, err := ms.Client.PollCompactionJobs(t.Context(), &metastorev1.PollCompactionJobsRequest{
			StatusUpdates: []*metastorev1.CompactionJobStatusUpdate{{
				Name: job.Name, Token: token, Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS,
				CompactedBlocks: &metastorev1.CompactedBlocks{
					SourceBlocks: &metastorev1.BlockList{Tenant: job.Tenant, Shard: job.Shard, Blocks: job.SourceBlocks},
					NewBlocks:    []*metastorev1.BlockMeta{result},
				},
			}},
		})
		require.NoError(t, err)
		return result
	}
	requireIdle := func() {
		// Planning and assignment take separate polls.
		for range 3 {
			response, err := ms.Client.PollCompactionJobs(t.Context(), &metastorev1.PollCompactionJobsRequest{JobCapacity: 1})
			require.NoError(t, err)
			require.Empty(t, response.CompactionJobs)
		}
	}

	for i := range ms.Instances {
		if i > 0 {
			ms.RestartInstance(i-1, supports(fsmversion.ConfigurableCompactionLevels))
		}
		requireActiveVersion(t, &ms, fsmversion.Baseline)
		result := complete(addSource("before"))
		require.Equal(t, uint32(3), result.CompactionLevel)
		requireIdle()
	}
	ms.RestartInstance(len(ms.Instances)-1, supports(fsmversion.ConfigurableCompactionLevels))
	requireActiveVersion(t, &ms, fsmversion.ConfigurableCompactionLevels)
	requireIdle() // Earlier terminal L3 blocks are not backfilled.

	result := complete(addSource("after"))
	require.Equal(t, uint32(3), result.CompactionLevel)
	// Snapshot the newly admitted L3 candidate and restore all replicas.
	requireSnapshotted(t, &ms, appliedIndex(t, &ms, leaderIndex(t, &ms)))
	for i := range ms.Instances {
		ms.RestartInstance(i, nil)
	}
	requireActiveVersion(t, &ms, fsmversion.ConfigurableCompactionLevels)
	result = complete(result)
	result = complete(result)
	require.Equal(t, uint32(5), result.CompactionLevel)
	requireIdle()
	for _, instance := range ms.Instances {
		response, err := instance.QueryMetadata(t.Context(), &metastorev1.QueryMetadataRequest{
			TenantId: []string{"after"}, StartTime: result.MinTime, EndTime: result.MaxTime, Query: `{}`,
		})
		require.NoError(t, err)
		require.Len(t, response.Blocks, 1)
		require.Equal(t, result.Id, response.Blocks[0].Id)
		require.Equal(t, uint32(5), response.Blocks[0].CompactionLevel)
	}
}
