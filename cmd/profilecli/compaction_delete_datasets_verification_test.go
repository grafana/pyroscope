package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestDeleteDatasetsIncomplete(t *testing.T) {
	for _, tt := range []struct {
		name      string
		matched   uint32
		jobs      []string
		remaining uint32
		want      string
	}{
		{name: "partial scheduling", matched: 2, jobs: []string{"job-1"}, want: "scheduled 1 of 2"},
		{name: "nothing scheduled", matched: 2, want: "scheduled 0 of 2"},
		{name: "evicted job", matched: 1, jobs: []string{"job-1"}, remaining: 1, want: "1 matching blocks remain"},
		{name: "verified completion", matched: 1, jobs: []string{"job-1"}},
		{name: "no matches"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := startFakeDeleteDatasetsServer(t, &metastorev1.DeleteDatasetsResponse{MatchedBlocksCount: tt.matched, ScheduledJobNames: tt.jobs})
			srv.verifyResp = &metastorev1.DeleteDatasetsResponse{DryRun: true, MatchedBlocksCount: tt.remaining}
			var buf bytes.Buffer
			err := deleteDatasets(withOutput(context.Background(), &buf), &deleteDatasetsParams{
				MetastoreAddress: srv.addr, Tenant: "tenant-a", From: "2026-10-16T08:00:00Z", To: "2026-10-16T09:00:00Z", ServiceNameRegex: "svc-a", DryRun: "false", Output: outputConsole,
			})
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				require.NotContains(t, buf.String(), "Deletion verified")
			} else {
				require.NoError(t, err)
				require.Contains(t, buf.String(), "Deletion verified")
			}
			require.NotContains(t, buf.String(), "All deletion jobs completed")
		})
	}
}
