package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type fakeDeleteDatasetsServer struct {
	metastorev1.UnimplementedCompactionServiceServer

	addr       string
	lastReq    *metastorev1.DeleteDatasetsRequest
	resp       *metastorev1.DeleteDatasetsResponse
	verifyResp *metastorev1.DeleteDatasetsResponse
}

func (s *fakeDeleteDatasetsServer) DeleteDatasets(_ context.Context, req *metastorev1.DeleteDatasetsRequest) (*metastorev1.DeleteDatasetsResponse, error) {
	s.lastReq = req
	if req.DryRun && s.verifyResp != nil {
		return s.verifyResp, nil
	}
	return s.resp, nil
}

func (s *fakeDeleteDatasetsServer) ListCompactionJobs(context.Context, *metastorev1.ListCompactionJobsRequest) (*metastorev1.ListCompactionJobsResponse, error) {
	return &metastorev1.ListCompactionJobsResponse{}, nil
}

func startFakeDeleteDatasetsServer(t *testing.T, resp *metastorev1.DeleteDatasetsResponse) *fakeDeleteDatasetsServer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &fakeDeleteDatasetsServer{
		addr: lis.Addr().String(),
		resp: resp,
	}
	grpcSrv := grpc.NewServer()
	metastorev1.RegisterCompactionServiceServer(grpcSrv, srv)

	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcSrv.Stop()
	})
	return srv
}

func TestDeleteDatasetsCommand(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	blockMin := time.Date(2026, 10, 16, 7, 30, 0, 0, time.UTC)
	blockMax := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)

	cannedResp := &metastorev1.DeleteDatasetsResponse{
		DryRun:                         true,
		MatchedBlocksCount:             2,
		PlannedFullBlockDeletionsCount: 1,
		PlannedBlockRewritesCount:      1,
		MatchedDatasetsCount:           3,
		MatchedBytes:                   4096,
		EffectiveMatchedMinTime:        blockMin.UnixNano(),
		EffectiveMatchedMaxTime:        blockMax.UnixNano(),
		Blocks: []*metastorev1.BlockDeletionSummary{
			{
				BlockId:                "01JABCDEF00000000000000001",
				Tenant:                 "tenant-a",
				Shard:                  1,
				CompactionLevel:        1,
				BlockMinTime:           blockMin.UnixNano(),
				BlockMaxTime:           blockMax.UnixNano(),
				MatchedMinTime:         blockMin.UnixNano(),
				MatchedMaxTime:         blockMax.UnixNano(),
				MatchedServices:        []string{"svc-a", "svc-b"},
				MatchedDatasetsCount:   2,
				RemainingDatasetsCount: 0,
				MatchedBytes:           2048,
				FullBlockPlanned:       true,
			},
			{
				BlockId:                "01JABCDEF00000000000000002",
				Tenant:                 "tenant-a",
				Shard:                  2,
				CompactionLevel:        2,
				BlockMinTime:           blockMin.UnixNano(),
				BlockMaxTime:           blockMax.UnixNano(),
				MatchedMinTime:         t1.UnixNano(),
				MatchedMaxTime:         t2.UnixNano(),
				MatchedServices:        []string{"svc-a"},
				MatchedDatasetsCount:   1,
				RemainingDatasetsCount: 2,
				MatchedBytes:           2048,
				FullBlockPlanned:       false,
			},
		},
		ScheduledJobNames: []string{"job-1", "job-2"},
	}

	srv := startFakeDeleteDatasetsServer(t, cannedResp)

	t.Run("console output", func(t *testing.T) {
		var buf bytes.Buffer
		ctx := withOutput(context.Background(), &buf)

		err := deleteDatasets(ctx, &deleteDatasetsParams{
			MetastoreAddress:   srv.addr,
			Tenant:             "tenant-a",
			From:               t1.Format(time.RFC3339),
			To:                 t2.Format(time.RFC3339),
			ServiceNameRegex:   "svc-a|svc-b",
			MinCompactionLevel: 2,
			DryRun:             "true",
			Output:             outputConsole,
		})
		require.NoError(t, err)
		require.NotNil(t, srv.lastReq)
		require.Equal(t, "tenant-a", srv.lastReq.Tenant)
		require.Equal(t, t1.UnixNano(), srv.lastReq.MinTime)
		require.Equal(t, t2.UnixNano(), srv.lastReq.MaxTime)
		require.Equal(t, "svc-a|svc-b", srv.lastReq.GetFilter().GetServiceNameRegex())
		require.Equal(t, uint32(2), srv.lastReq.MinCompactionLevel)
		require.True(t, srv.lastReq.DryRun)

		out := buf.String()
		require.Contains(t, out, "DRY-RUN")
		require.Contains(t, out, "01JABCDEF00000000000000001")
		require.Contains(t, out, "delete-block")
		require.Contains(t, out, "01JABCDEF00000000000000002")
		require.Contains(t, out, "rewrite")
		require.Contains(t, out, "job-1")
	})

	t.Run("json output", func(t *testing.T) {
		srv.verifyResp = &metastorev1.DeleteDatasetsResponse{DryRun: true}
		defer func() { srv.verifyResp = nil }()
		var buf bytes.Buffer
		ctx := withOutput(context.Background(), &buf)

		err := deleteDatasets(ctx, &deleteDatasetsParams{
			MetastoreAddress: srv.addr,
			Tenant:           "tenant-a",
			From:             t1.Format(time.RFC3339),
			To:               t2.Format(time.RFC3339),
			ServiceNameRegex: "svc-a",
			DryRun:           "false",
			Output:           outputJSON,
		})
		require.NoError(t, err)
		require.True(t, srv.lastReq.DryRun, "verify deletion with a dry run")

		var parsed metastorev1.DeleteDatasetsResponse
		dec := json.NewDecoder(&buf)
		require.NoError(t, dec.Decode(&parsed))
		require.ErrorIs(t, dec.Decode(new(any)), io.EOF)
		require.EqualValues(t, 2, parsed.MatchedBlocksCount)
		require.Len(t, parsed.Blocks, 2)
	})

	t.Run("invalid time range", func(t *testing.T) {
		err := deleteDatasets(context.Background(), &deleteDatasetsParams{
			MetastoreAddress: srv.addr,
			Tenant:           "tenant-a",
			From:             t2.Format(time.RFC3339),
			To:               t1.Format(time.RFC3339),
			ServiceNameRegex: "svc-a",
			DryRun:           "true",
			Output:           outputConsole,
		})
		require.ErrorContains(t, err, "cannot be after")
	})
}
