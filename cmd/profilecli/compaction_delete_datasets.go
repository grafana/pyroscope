package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/operations"
)

type deleteDatasetsParams struct {
	MetastoreAddress   string
	Tenant             string
	From               string
	To                 string
	ServiceNameRegex   string
	MinCompactionLevel uint32
	DryRun             string
	Output             string
}

func addDeleteDatasetsParams(cmd commander) *deleteDatasetsParams {
	params := &deleteDatasetsParams{}
	cmd.Flag("metastore.address", "Address of the metastore (host:port). Accepts a comma-separated list of peers.").
		Default("localhost:9095").StringVar(&params.MetastoreAddress)
	cmd.Flag("tenant-id", "Tenant ID whose matching service datasets should be deleted.").
		Required().StringVar(&params.Tenant)
	cmd.Flag("from", "Beginning of the deletion window (inclusive), e.g. RFC3339 or relative like now-1h.").
		Required().StringVar(&params.From)
	cmd.Flag("to", "End of the deletion window (inclusive), e.g. RFC3339 or relative like now.").
		Required().StringVar(&params.To)
	cmd.Flag("service-name-regex", "Full-string RE2 regular expression matching service_name datasets to delete (e.g. \"svc-a|svc-b\").").
		Required().StringVar(&params.ServiceNameRegex)
	cmd.Flag("min-compaction-level", "Minimum compaction level of blocks to consider.").
		Default("0").Uint32Var(&params.MinCompactionLevel)
	cmd.Flag("dry-run", "Inspect the impact of dataset deletion without scheduling rewrite/delete jobs.").
		Default("true").EnumVar(&params.DryRun, "true", "false")
	cmd.Flag("output", "Output format: console or json.").
		Default(outputConsole).EnumVar(&params.Output, outputConsole, outputJSON)
	return params
}

func (p *deleteDatasetsParams) parseFromTo() (from, to time.Time, err error) {
	from, err = operations.ParseTime(p.From)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse from: %w", err)
	}
	to, err = operations.ParseTime(p.To)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse to: %w", err)
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("from (%s) cannot be after to (%s)", from, to)
	}
	return from, to, nil
}

func deleteDatasets(ctx context.Context, params *deleteDatasetsParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	mc, err := newMetastoreClient(ctx, params.MetastoreAddress)
	if err != nil {
		return err
	}
	defer func() {
		_ = services.StopAndAwaitTerminated(context.Background(), mc.Service())
	}()

	req := &metastorev1.DeleteDatasetsRequest{
		Tenant:             params.Tenant,
		MinTime:            from.UnixNano(),
		MaxTime:            to.UnixNano(),
		Filter:             &metastorev1.DeleteFilter{ServiceNameRegex: params.ServiceNameRegex},
		DryRun:             params.DryRun != "false",
		MinCompactionLevel: params.MinCompactionLevel,
	}

	resp, err := mc.DeleteDatasets(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to delete datasets: %w", err)
	}

	if err := outputDeleteDatasets(ctx, req, resp, params.Output); err != nil {
		return err
	}
	if !req.DryRun && len(resp.ScheduledJobNames) > 0 {
		return watchDeletionJobs(ctx, mc.ListCompactionJobs, params.Tenant, resp.ScheduledJobNames, params.Output, time.Second)
	}
	return nil
}

// Completed jobs are removed from the persisted list. Failures remain visible
// while the scheduler retries them; stop watching on the first observed failure.
func watchDeletionJobs(ctx context.Context, list func(context.Context, *metastorev1.ListCompactionJobsRequest, ...grpc.CallOption) (*metastorev1.ListCompactionJobsResponse, error), tenant string, names []string, format string, interval time.Duration) error {
	pending := make(map[string]struct{}, len(names))
	for _, name := range names {
		pending[name] = struct{}{}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := list(ctx, &metastorev1.ListCompactionJobsRequest{Tenant: tenant})
		if err != nil {
			return fmt.Errorf("failed to watch deletion jobs: %w", err)
		}
		active := make(map[string]struct{})
		for _, info := range resp.Jobs {
			name := info.GetJob().GetName()
			if _, ok := pending[name]; !ok {
				continue
			}
			if info.Failures > 0 {
				return fmt.Errorf("deletion job %s has %d failure(s); it may still be retried by the scheduler", name, info.Failures)
			}
			if info.Status != metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS {
				active[name] = struct{}{}
			}
		}
		for name := range pending {
			if _, ok := active[name]; !ok {
				if format != outputJSON {
					fmt.Fprintf(output(ctx), "Job %s completed.\n", name)
				}
				delete(pending, name)
			}
		}
		if len(pending) == 0 {
			if format != outputJSON {
				fmt.Fprintln(output(ctx), "All deletion jobs completed.")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func outputDeleteDatasets(ctx context.Context, req *metastorev1.DeleteDatasetsRequest, resp *metastorev1.DeleteDatasetsResponse, format string) error {
	if format == outputJSON {
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	out := output(ctx)
	mode := "EXECUTE"
	if req.DryRun {
		mode = "DRY-RUN (pass --dry-run=false to schedule deletion jobs)"
	}

	fmt.Fprintf(out, "Mode:                     %s\n", mode)
	fmt.Fprintf(out, "Tenant:                   %s\n", req.Tenant)
	fmt.Fprintf(out, "Service Name Regex:       %s\n", req.GetFilter().GetServiceNameRegex())
	fmt.Fprintf(out, "Min Compaction Level:     L%d\n", req.MinCompactionLevel)
	fmt.Fprintf(out, "Requested Window:         %s .. %s\n",
		time.Unix(0, req.MinTime).UTC().Format(time.RFC3339),
		time.Unix(0, req.MaxTime).UTC().Format(time.RFC3339),
	)
	fmt.Fprintf(out, "Matched Blocks:           %d (%d full block deletions, %d block rewrites)\n",
		resp.MatchedBlocksCount, resp.PlannedFullBlockDeletionsCount, resp.PlannedBlockRewritesCount)
	fmt.Fprintf(out, "Matched Datasets:         %d (%s)\n",
		resp.MatchedDatasetsCount, humanize.Bytes(resp.MatchedBytes))

	if resp.MatchedBlocksCount > 0 {
		fmt.Fprintf(out, "Effective Matched Window: %s .. %s\n",
			time.Unix(0, resp.EffectiveMatchedMinTime).UTC().Format(time.RFC3339),
			time.Unix(0, resp.EffectiveMatchedMaxTime).UTC().Format(time.RFC3339),
		)
		fmt.Fprintln(out)

		table := newTableWriter(out)
		table.SetHeader([]string{
			"Block ID",
			"Shard",
			"Level",
			"Action",
			"Matched / Remaining",
			"Matched Size",
			"Block Window",
			"Matched Dataset Window",
			"Matched Services",
		})
		for _, b := range resp.Blocks {
			action := "rewrite"
			if b.FullBlockPlanned {
				action = "delete-block"
			}
			blockWindow := fmt.Sprintf("%s .. %s",
				time.Unix(0, b.BlockMinTime).UTC().Format(time.RFC3339),
				time.Unix(0, b.BlockMaxTime).UTC().Format(time.RFC3339),
			)
			deletedWindow := fmt.Sprintf("%s .. %s",
				time.Unix(0, b.MatchedMinTime).UTC().Format(time.RFC3339),
				time.Unix(0, b.MatchedMaxTime).UTC().Format(time.RFC3339),
			)
			table.Append([]string{
				b.BlockId,
				strconv.FormatUint(uint64(b.Shard), 10),
				fmt.Sprintf("L%d", b.CompactionLevel),
				action,
				fmt.Sprintf("%d / %d", b.MatchedDatasetsCount, b.RemainingDatasetsCount),
				humanize.Bytes(b.MatchedBytes),
				blockWindow,
				deletedWindow,
				strings.Join(b.MatchedServices, ", "),
			})
		}
		table.Render()
	}

	if len(resp.ScheduledJobNames) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Scheduled %d compaction job(s):\n", len(resp.ScheduledJobNames))
		for _, name := range resp.ScheduledJobNames {
			fmt.Fprintf(out, " - %s\n", name)
		}
	}

	return nil
}
