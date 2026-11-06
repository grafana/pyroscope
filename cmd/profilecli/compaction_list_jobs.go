package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/grafana/dskit/services"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type listCompactionJobsParams struct {
	MetastoreAddress string
	Tenant           string
	Output           string
}

func addListCompactionJobsParams(cmd commander) *listCompactionJobsParams {
	params := &listCompactionJobsParams{}
	cmd.Flag("metastore.address", "Address of the metastore (host:port). Accepts a comma-separated list of peers.").
		Default("localhost:9095").StringVar(&params.MetastoreAddress)
	cmd.Flag("tenant-id", "Filter jobs by tenant; defaults to all tenants.").StringVar(&params.Tenant)
	cmd.Flag("output", "Output format: console or json.").
		Default(outputConsole).EnumVar(&params.Output, outputConsole, outputJSON)
	return params
}

func listCompactionJobs(ctx context.Context, params *listCompactionJobsParams) error {
	mc, err := newMetastoreClient(ctx, params.MetastoreAddress)
	if err != nil {
		return err
	}
	defer func() {
		_ = services.StopAndAwaitTerminated(context.Background(), mc.Service())
	}()
	resp, err := mc.ListCompactionJobs(ctx, &metastorev1.ListCompactionJobsRequest{Tenant: params.Tenant})
	if err != nil {
		return fmt.Errorf("failed to list compaction jobs: %w", err)
	}
	return outputCompactionJobs(ctx, resp, params.Output)
}

func outputCompactionJobs(ctx context.Context, resp *metastorev1.ListCompactionJobsResponse, format string) error {
	if format == outputJSON {
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
	table := newTableWriter(output(ctx))
	table.SetHeader([]string{"Name", "Tenant", "Shard", "Level", "Status", "Blocks", "Failures", "Deletion"})
	for _, info := range resp.Jobs {
		j := info.Job
		state := "queued"
		if info.Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS {
			state = "assigned"
		}
		table.Append([]string{
			j.Name,
			j.Tenant,
			strconv.FormatUint(uint64(j.Shard), 10),
			fmt.Sprintf("L%d", j.CompactionLevel),
			state,
			strconv.Itoa(len(j.SourceBlocks)),
			fmt.Sprint(info.Failures),
			strconv.FormatBool(j.DeleteFilter != nil),
		})
	}
	table.Render()
	return nil
}
