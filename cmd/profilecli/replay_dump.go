package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
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
	ReplayBucket *bucketParams

	MetastoreAddress string
	Tenant           string
	Query            string
	From             string
	To               string
	Output           string
	OutputObject     string
	SkipIfExists     bool
	Force            bool
	AnonymizeSalt    string
}

func addReplayDumpParams(cmd commander) *replayDumpParams {
	params := &replayDumpParams{}
	params.bucketParams = addBucketParams(cmd)
	params.ReplayBucket = addReplayBucketParams(cmd)

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
	cmd.Flag("output", "Path to write the replay dump file to. Exactly one of --output and --output-object is required.").Short('o').StringVar(&params.Output)
	cmd.Flag("output-object", "Object name to write the replay dump to in the bucket configured with the --replay.storage.* flags. Exactly one of --output and --output-object is required.").StringVar(&params.OutputObject)
	cmd.Flag("skip-if-exists", "Exit successfully without dumping if the output already exists.").BoolVar(&params.SkipIfExists)
	cmd.Flag("force", "Overwrite the output file if it already exists.").Short('f').Default("false").BoolVar(&params.Force)
	cmd.Flag("anonymize-salt", "Anonymize labels and symbols with this salt (64-character SHA-256 hex hashes); preserve profile-type labels. An empty salt disables anonymization.").StringVar(&params.AnonymizeSalt)
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
type flagPrefixCommander struct {
	commander
	prefix string
}

func (c flagPrefixCommander) Flag(name, help string) *kingpin.FlagClause {
	return c.commander.Flag(c.prefix+name, help)
}

func addReplayBucketParams(cmd commander) *bucketParams {
	return addBucketParams(flagPrefixCommander{commander: cmd, prefix: "replay."})
}

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
	if (params.Output == "") == (params.OutputObject == "") {
		return errors.New("exactly one of --output and --output-object is required")
	}
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	matchers, err := phlaremodel.ParseMetricSelector(params.Query)
	if err != nil {
		return fmt.Errorf("failed to parse query: %w", err)
	}

	anonymizer := replayAnonymizer(params.AnonymizeSalt)
	header := anonymizer.header(replayHeader{
		SourceQuery: params.Query,
		Tenants:     []string{params.Tenant},
		From:        from.UnixMilli(),
		To:          to.UnixMilli(),
		CreatedAt:   time.Now().UnixMilli(),
	})
	level.Info(logger).Log("msg", "starting replay dump",
		"metastore", params.MetastoreAddress, "tenant", params.Tenant,
		"query", params.Query, "from", from, "to", to, "output", params.Output)

	var replayBucket phlareobj.Bucket
	if params.OutputObject != "" {
		replayBucket, err = params.ReplayBucket.initClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create replay bucket client: %w", err)
		}
	}
	exists, err := replayDumpExists(ctx, params, replayBucket)
	if err != nil {
		return err
	}
	if exists {
		if params.SkipIfExists {
			level.Info(logger).Log("msg", "replay dump already exists, skipping", "output", params.Output, "object", params.OutputObject)
			return nil
		}
		if !params.Force {
			return fmt.Errorf("output %s already exists (use --force to overwrite or --skip-if-exists to skip)", params.Output+params.OutputObject)
		}
	}

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

	// Write to a temporary file alongside the destination and atomically
	// rename it into place only on success, so a failed or interrupted dump
	// never leaves a partial/corrupt file at the destination path.
	outDir, outName := filepath.Dir(params.Output), filepath.Base(params.Output)
	if replayBucket != nil {
		outDir, outName = os.TempDir(), filepath.Base(params.OutputObject)
	}
	f, err := os.CreateTemp(outDir, outName+".tmp-*")
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

	rw, err := newReplayWriter(f, header)
	if err != nil {
		return fmt.Errorf("failed to write replay header: %w", err)
	}

	startNanos := from.UnixNano()
	endNanos := to.UnixNano()

	totalProfiles, err := dumpBlocks(ctx, bucket, resp.Blocks, matchers, startNanos, endNanos, rw, outDir, anonymizer)
	if err != nil {
		return err
	}

	if err := rw.Flush(); err != nil {
		return fmt.Errorf("failed to flush replay dump: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file: %w", err)
	}
	if replayBucket != nil {
		if err := uploadReplayDump(ctx, replayBucket, params.OutputObject, tmpName); err != nil {
			return err
		}
	} else {
		if err := os.Rename(tmpName, params.Output); err != nil {
			return fmt.Errorf("failed to move dump into place: %w", err)
		}
		renamed = true
	}

	level.Info(logger).Log("msg", "replay dump complete",
		"blocks", len(resp.Blocks), "profiles", totalProfiles, "output", params.Output)
	return nil
}

func replayDumpExists(ctx context.Context, params *replayDumpParams, replayBucket phlareobj.Bucket) (bool, error) {
	if replayBucket != nil {
		exists, err := replayBucket.Exists(ctx, params.OutputObject)
		if err != nil {
			return false, fmt.Errorf("failed to check replay object %s: %w", params.OutputObject, err)
		}
		return exists, nil
	}
	if _, err := os.Stat(params.Output); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("failed to stat output file: %w", err)
	}
	return false, nil
}

func uploadReplayDump(ctx context.Context, bucket phlareobj.Bucket, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open dump for upload: %w", err)
	}
	defer f.Close()
	if err := bucket.Upload(ctx, name, f); err != nil {
		return fmt.Errorf("failed to upload replay dump to %s: %w", name, err)
	}
	return nil
}

func dumpBlock(
	ctx context.Context,
	bucket phlareobj.Bucket,
	md *metastorev1.BlockMeta,
	matchers []*labels.Matcher,
	startNanos, endNanos int64,
	rw replayRecordWriter,
	anonymizer replayAnonymizer,
) (int, error) {
	obj := block.NewObject(bucket, md)

	var count int
	for _, dsMeta := range md.Datasets {
		if block.DatasetFormat(dsMeta.Format) != block.DatasetFormat0 {
			// Skip tenant-wide dataset index entries: they do not carry
			// profile/tsdb/symbol sections of their own.
			continue
		}
		n, err := dumpDataset(ctx, obj, dsMeta, matchers, startNanos, endNanos, rw, anonymizer)
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
	rw replayRecordWriter,
	anonymizer replayAnonymizer,
) (int, error) {
	ds := block.NewDataset(dsMeta, obj)
	if err := ds.Open(ctx, block.SectionTSDB, block.SectionProfiles, block.SectionSymbols); err != nil {
		return 0, fmt.Errorf("failed to open dataset: %w", err)
	}

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		_ = ds.Close()
		return 0, fmt.Errorf("failed to create profile row iterator: %w", err)
	}
	defer it.Close()

	// Keep each full symbol partition (including its stacktrace trees) loaded
	// until all rows have been processed. Rows are series-, not partition-ordered.
	symbols := &replaySymbols{source: ds.Symbols(), partitions: make(map[uint64]symdb.PartitionReader), anonymizer: anonymizer}
	defer symbols.Close()

	var count int
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		entry := it.At()

		if entry.Timestamp < startNanos || entry.Timestamp > endNanos {
			continue
		}
		if !matchesAll(matchers, entry.Labels) {
			continue
		}

		pprofBytes, err := buildPprofForRow(ctx, symbols, entry)
		if err != nil {
			// Skip individual profiles that fail to reconstruct/marshal rather
			// than aborting the whole dump: a best-effort dump of the
			// remaining profiles is more useful than none.
			level.Warn(logger).Log("msg", "skipping profile that failed to reconstruct",
				"timestamp", entry.Timestamp, "labels", anonymizer.labels(entry.Labels).ToPrometheusLabels().String(), "err", err)
			continue
		}

		if err := rw.WriteRecord(replayRecord{
			Labels:         anonymizer.labels(entry.Labels),
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
func buildPprofForRow(ctx context.Context, symbols symdb.SymbolsReader, entry block.ProfileEntry) ([]byte, error) {
	resolver := symdb.NewResolver(ctx, symbols)
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
