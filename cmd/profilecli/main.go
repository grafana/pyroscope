package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	_ "github.com/grafana/pyroscope/pkg/util/build"
)

var cfg struct {
	verbose bool
	blocks  struct {
		path               string
		restoreMissingMeta bool
		compact            struct {
			src    string
			dst    string
			shards int
		}
	}
}

var (
	consoleOutput = os.Stderr
	logger        = log.NewLogfmtLogger(consoleOutput)
)

func main() {
	ctx := phlarecontext.WithLogger(context.Background(), logger)
	ctx = withOutput(ctx, os.Stdout)

	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for Grafana Pyroscope, the continuous profiling aggregation system.").UsageWriter(os.Stdout)
	app.Version(version.Print("profilecli"))
	app.HelpFlag.Short('h')
	app.Flag("verbose", "Enable verbose logging.").Short('v').Default("0").BoolVar(&cfg.verbose)

	adminCmd := app.Command("admin", "Administrative tasks for Pyroscope cluster operators.")

	blocksCmd := adminCmd.Command("blocks", "Operate on Grafana Pyroscope's blocks.")
	blocksCmd.Flag("path", "Path to blocks directory").Default("./data/local").StringVar(&cfg.blocks.path)

	blocksListCmd := blocksCmd.Command("list", "List blocks.")
	blocksListCmd.Flag("restore-missing-meta", "").Default("false").BoolVar(&cfg.blocks.restoreMissingMeta)

	blocksCompactCmd := blocksCmd.Command("compact", "Compact blocks.")
	blocksCompactCmd.Arg("from", "The source input blocks to compact.").Required().ExistingDirVar(&cfg.blocks.compact.src)
	blocksCompactCmd.Arg("dest", "The destination where compacted blocks should be stored.").Required().StringVar(&cfg.blocks.compact.dst)
	blocksCompactCmd.Flag("shards", "The amount of shards to split output blocks into.").Default("0").IntVar(&cfg.blocks.compact.shards)

	parquetCmd := adminCmd.Command("parquet", "Operate on a Parquet file.")
	parquetInspectCmd := parquetCmd.Command("inspect", "Inspect a parquet file's structure.")
	parquetInspectFiles := parquetInspectCmd.Arg("file", "parquet file path").Required().ExistingFiles()

	queryCmd := app.Command("query", "Query profile store.")
	queryMergeCmd := queryCmd.Command("merge", "Request merged profile.")
	queryMergeOutput := queryMergeCmd.Flag("output", "How to output the result, examples: console, raw, pprof=./my.pprof").Default("console").String()
	queryMergeParams := addQueryMergeParams(queryMergeCmd)
	querySeriesCmd := queryCmd.Command("series", "Request series labels.")
	querySeriesParams := addQuerySeriesParams(querySeriesCmd)

	queryTracerCmd := app.Command("query-tracer", "Analyze query traces.")
	queryTracerParams := addQueryTracerParams(queryTracerCmd)

	uploadCmd := app.Command("upload", "Upload profile(s).")
	uploadParams := addUploadParams(uploadCmd)

	canaryExporterCmd := app.Command("canary-exporter", "Run the canary exporter.")
	canaryExporterParams := addCanaryExporterParams(canaryExporterCmd)

	bucketCmd := adminCmd.Command("bucket", "Run the bucket visualization tool.")
	bucketWebCmd := bucketCmd.Command("web", "Run the web tool for visualizing blocks in object-store buckets.")
	bucketWebParams := addBucketWebToolParams(bucketWebCmd)

	// parse command line arguments
	parsedCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	// enable verbose logging if requested
	if !cfg.verbose {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	switch parsedCmd {
	case blocksListCmd.FullCommand():
		os.Exit(checkError(blocksList(ctx)))
	case parquetInspectCmd.FullCommand():
		for _, file := range *parquetInspectFiles {
			if err := parquetInspect(ctx, file); err != nil {
				os.Exit(checkError(err))
			}
		}
	case queryMergeCmd.FullCommand():
		if err := queryMerge(ctx, queryMergeParams, *queryMergeOutput); err != nil {
			os.Exit(checkError(err))
		}
	case querySeriesCmd.FullCommand():
		if err := querySeries(ctx, querySeriesParams); err != nil {
			os.Exit(checkError(err))
		}

	case queryTracerCmd.FullCommand():
		if err := queryTracer(ctx, queryTracerParams); err != nil {
			os.Exit(checkError(err))
		}

	case uploadCmd.FullCommand():
		if err := upload(ctx, uploadParams); err != nil {
			os.Exit(checkError(err))
		}
	case canaryExporterCmd.FullCommand():
		if err := newCanaryExporter(canaryExporterParams).run(ctx); err != nil {
			os.Exit(checkError(err))
		}
	case bucketWebCmd.FullCommand():
		if err := newBucketWebTool(bucketWebParams).run(ctx); err != nil {
			os.Exit(checkError(err))
		}
	case blocksCompactCmd.FullCommand():
		if err := blocksCompact(ctx, cfg.blocks.compact.src, cfg.blocks.compact.dst, cfg.blocks.compact.shards); err != nil {
			os.Exit(checkError(err))
		}
	default:
		level.Error(logger).Log("msg", "unknown command", "cmd", parsedCmd)
	}
}

func checkError(err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

type contextKey uint8

const (
	contextKeyOutput contextKey = iota
)

func withOutput(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, contextKeyOutput, w)
}

func output(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(contextKeyOutput).(io.Writer); ok {
		return w
	}
	return os.Stdout
}
