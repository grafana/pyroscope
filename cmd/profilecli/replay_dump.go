package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/prometheus/model/labels"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	metastoreclient "github.com/grafana/pyroscope/v2/pkg/metastore/client"
	"github.com/grafana/pyroscope/v2/pkg/metastore/discovery"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/operations"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

type replayDumpParams struct {
	*bucketParams

	MetastoreAddress string
	Tenant           string
	Query            string
	From             string
	To               string
	Output           string
	Force            bool
}

func addReplayDumpParams(cmd commander) *replayDumpParams {
	params := &replayDumpParams{}
	params.bucketParams = addBucketParams(cmd)

	cmd.Flag("metastore.address", "Address of the source cell's metastore (host:port). Accepts a comma-separated list of peers.").
		Default("localhost:9095").StringVar(&params.MetastoreAddress)
	// NOTE: only a single tenant is supported for now. The replay push side
	// sends everything to one destination tenant (X-Scope-OrgID), so dumping
	// multiple tenants at once would silently merge them into one on replay.
	cmd.Flag("tenant-id", "Tenant ID to dump from the source cell. Only a single tenant is supported for now.").
		Required().StringVar(&params.Tenant)
	cmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	cmd.Flag("from", "Beginning of the dump window.").Default("now-1h").StringVar(&params.From)
	cmd.Flag("to", "End of the dump window.").Default("now").StringVar(&params.To)
	cmd.Flag("output", "Path to write the replay dump file to.").Short('o').Required().StringVar(&params.Output)
	cmd.Flag("force", "Overwrite the output file if it already exists.").Short('f').Default("false").BoolVar(&params.Force)
	return params
}

func (p *replayDumpParams) parseFromTo() (from, to time.Time, err error) {
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

// newMetastoreClient builds a metastore client from a comma-separated list of
// addresses. It relies on plaintext (non-TLS) gRPC, matching the default
// configuration of the metastore.
func newMetastoreClient(ctx context.Context, address string) (*metastoreclient.Client, error) {
	disc, err := discovery.NewDiscovery(logger, address, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metastore discovery: %w", err)
	}

	// Populate default gRPC client settings (e.g. max message sizes), as the
	// zero-value grpcclient.Config has them disabled (set to 0).
	var grpcClientConfig grpcclient.Config
	grpcClientConfig.RegisterFlags(flag.NewFlagSet("", flag.ContinueOnError))

	client := metastoreclient.New(logger, grpcClientConfig, disc)
	if err := services.StartAndAwaitRunning(ctx, client.Service()); err != nil {
		return nil, fmt.Errorf("failed to start metastore client: %w", err)
	}
	return client, nil
}

func replayDump(ctx context.Context, params *replayDumpParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	matchers, err := phlaremodel.ParseMetricSelector(params.Query)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}

	level.Info(logger).Log("msg", "starting replay dump",
		"metastore", params.MetastoreAddress, "tenant", params.Tenant,
		"query", params.Query, "from", from, "to", to, "output", params.Output)

	bucket, err := params.initClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create bucket client: %w", err)
	}

	mc, err := newMetastoreClient(ctx, params.MetastoreAddress)
	if err != nil {
		return err
	}
	defer func() {
		_ = services.StopAndAwaitTerminated(context.Background(), mc.Service())
	}()

	resp, err := mc.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  []string{params.Tenant},
		StartTime: from.UnixMilli(),
		EndTime:   to.UnixMilli(),
		Query:     params.Query,
	})
	if err != nil {
		return fmt.Errorf("failed to query metastore: %w", err)
	}
	level.Info(logger).Log("msg", "found blocks matching query", "count", len(resp.Blocks))

	// Fail early if the destination exists and we are not overwriting, before
	// doing the (potentially expensive) dump work.
	if !params.Force {
		if _, statErr := os.Stat(params.Output); statErr == nil {
			return fmt.Errorf("output file %s already exists (use --force to overwrite)", params.Output)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("failed to stat output file: %w", statErr)
		}
	}

	// Write to a temporary file alongside the destination and atomically
	// rename it into place only on success, so a failed or interrupted dump
	// never leaves a partial/corrupt file at the destination path.
	f, err := os.CreateTemp(filepath.Dir(params.Output), filepath.Base(params.Output)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary output file: %w", err)
	}
	tmpName := f.Name()
	renamed := false
	defer func() {
		_ = f.Close()
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	rw, err := newReplayWriter(f, replayHeader{
		SourceQuery: params.Query,
		Tenants:     []string{params.Tenant},
		From:        from.UnixMilli(),
		To:          to.UnixMilli(),
		CreatedAt:   time.Now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("failed to write replay header: %w", err)
	}

	startNanos := from.UnixNano()
	endNanos := to.UnixNano()

	var totalProfiles int
	for _, md := range resp.Blocks {
		n, dErr := dumpBlock(ctx, bucket, md, matchers, startNanos, endNanos, rw)
		if dErr != nil {
			return fmt.Errorf("failed to dump block %s: %w", md.Id, dErr)
		}
		totalProfiles += n
		level.Debug(logger).Log("msg", "dumped block", "block", md.Id, "profiles", n)
	}

	if err := rw.Flush(); err != nil {
		return fmt.Errorf("failed to flush replay dump: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file: %w", err)
	}
	if err := os.Rename(tmpName, params.Output); err != nil {
		return fmt.Errorf("failed to move dump into place: %w", err)
	}
	renamed = true

	level.Info(logger).Log("msg", "replay dump complete",
		"blocks", len(resp.Blocks), "profiles", totalProfiles, "output", params.Output)
	return nil
}

func dumpBlock(
	ctx context.Context,
	bucket phlareobj.Bucket,
	md *metastorev1.BlockMeta,
	matchers []*labels.Matcher,
	startNanos, endNanos int64,
	rw *replayWriter,
) (int, error) {
	obj := block.NewObject(bucket, md)

	var count int
	for _, dsMeta := range md.Datasets {
		if block.DatasetFormat(dsMeta.Format) != block.DatasetFormat0 {
			// Skip tenant-wide dataset index entries: they do not carry
			// profile/tsdb/symbol sections of their own.
			continue
		}
		n, err := dumpDataset(ctx, obj, dsMeta, matchers, startNanos, endNanos, rw)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

func dumpDataset(
	ctx context.Context,
	obj *block.Object,
	dsMeta *metastorev1.Dataset,
	matchers []*labels.Matcher,
	startNanos, endNanos int64,
	rw *replayWriter,
) (int, error) {
	ds := block.NewDataset(dsMeta, obj)
	if err := ds.Open(ctx, block.SectionTSDB, block.SectionProfiles, block.SectionSymbols); err != nil {
		return 0, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		return 0, fmt.Errorf("failed to create profile row iterator: %w", err)
	}
	defer it.Close()

	var count int
	for it.Next() {
		entry := it.At()

		if entry.Timestamp < startNanos || entry.Timestamp > endNanos {
			continue
		}
		if !matchesAll(matchers, entry.Labels) {
			continue
		}

		pprofBytes, err := buildPprofForRow(ctx, ds, entry)
		if err != nil {
			// Skip individual profiles that fail to reconstruct/marshal rather
			// than aborting the whole dump: a best-effort dump of the
			// remaining profiles is more useful than none.
			level.Warn(logger).Log("msg", "skipping profile that failed to reconstruct",
				"timestamp", entry.Timestamp, "labels", entry.Labels.ToPrometheusLabels().String(), "err", err)
			continue
		}

		if err := rw.WriteRecord(replayRecord{
			Labels:         entry.Labels,
			TimestampNanos: entry.Timestamp,
			Pprof:          pprofBytes,
		}); err != nil {
			return count, fmt.Errorf("failed to write replay record: %w", err)
		}
		count++
	}
	if err := it.Err(); err != nil {
		return count, fmt.Errorf("failed to iterate profiles: %w", err)
	}
	return count, nil
}

func matchesAll(matchers []*labels.Matcher, ls phlaremodel.Labels) bool {
	for _, m := range matchers {
		if !m.Matches(ls.Get(m.Name)) {
			return false
		}
	}
	return true
}

// buildPprofForRow reconstructs a standalone pprof profile (gzip-compressed
// bytes) from a single profile row, resolving its stack traces via the
// dataset's symbol table. A new symdb.Resolver is used per profile, as
// documented on symdb.Resolver.
func buildPprofForRow(ctx context.Context, ds *block.Dataset, entry block.ProfileEntry) ([]byte, error) {
	resolver := symdb.NewResolver(ctx, ds.Symbols())
	defer resolver.Release()

	entry.Row.ForStacktraceIdsAndValues(func(stacktraceIDs, values []parquet.Value) {
		resolver.AddSamplesFromParquetRow(entry.Row.StacktracePartitionID(), stacktraceIDs, values)
	})

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, err
	}

	if profileType := entry.Labels.Get(phlaremodel.LabelNameProfileType); profileType != "" {
		if t, err := phlaremodel.ParseProfileTypeSelector(profileType); err == nil {
			pprof.SetProfileMetadata(profile, t, entry.Timestamp, 0)
		}
	}
	profile.TimeNanos = entry.Timestamp

	data, err := pprof.Marshal(profile, true)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reconstructed pprof: %w", err)
	}
	return data, nil
}
