package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestOutputCompactionJobs(t *testing.T) {
	t.Parallel()

	resp := &metastorev1.ListCompactionJobsResponse{
		Jobs: []*metastorev1.CompactionJobInfo{
			{
				Job: &metastorev1.CompactionJob{
					Name: "deletion-job", Tenant: "tenant-a", Shard: 2, CompactionLevel: 1,
					DeleteFilter: &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a"},
				},
				Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			},
			{Job: &metastorev1.CompactionJob{Name: "queued-job", Tenant: "tenant-b"}},
		},
	}

	t.Run("console", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputCompactionJobs(withOutput(context.Background(), &buf), resp, outputConsole))
		for _, want := range []string{"Name", "Tenant", "deletion-job", "tenant-a", "L1", "assigned", "true", "queued-job", "queued"} {
			require.Contains(t, buf.String(), want)
		}
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputCompactionJobs(withOutput(context.Background(), &buf), resp, outputJSON))
		var got metastorev1.ListCompactionJobsResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
		require.Equal(t, resp, &got)
	})

	t.Run("empty", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputCompactionJobs(withOutput(context.Background(), &buf), &metastorev1.ListCompactionJobsResponse{}, outputConsole))
	})
}
