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

	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	_ "github.com/grafana/phlare/pkg/util/build"
)

var cfg struct {
	verbose bool
	blocks  struct {
		path               string
		restoreMissingMeta bool
	}
}

var (
	consoleOutput = os.Stderr
	logger        = log.NewLogfmtLogger(consoleOutput)
)

func main() {
	ctx := phlarecontext.WithLogger(context.Background(), logger)
	ctx = withOutput(ctx, os.Stdout)

	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for Grafana Phlare, the continuous profiling aggregation system.").UsageWriter(os.Stdout)
	app.Version(version.Print("phlaretool"))
	app.HelpFlag.Short('h')
	app.Flag("verbose", "Enable verbose logging.").Short('v').Default("0").BoolVar(&cfg.verbose)

	blocksCmd := app.Command("blocks", "Operate on Grafana Phlare's blocks.")
	blocksCmd.Flag("path", "Path to blocks directory").Default("./data/local").StringVar(&cfg.blocks.path)

	blocksListCmd := blocksCmd.Command("list", "List blocks.")
	blocksListCmd.Flag("restore-missing-meta", "").Default("false").BoolVar(&cfg.blocks.restoreMissingMeta)

	parquetCmd := app.Command("parquet", "Operate on a Parquet file.")
	parquetInspectCmd := parquetCmd.Command("inspect", "Inspect a parquet file's structure.")
	parquetInspectFiles := parquetInspectCmd.Arg("file", "parquet file path").Required().ExistingFiles()

	queryCmd := app.Command("query", "Query profile store.")
	queryParams := addQueryParams(queryCmd)
	queryOutput := queryCmd.Flag("output", "How to output the result, examples: console, raw, pprof=./my.pprof").Default("console").String()
	queryMergeCmd := queryCmd.Command("merge", "Request merged profile.")

	uploadCmd := app.Command("upload", "Upload profile(s).")
	uploadParams := addUploadParams(uploadCmd)

	canaryExporterCmd := app.Command("canary-exporter", "Run the canary exporter.")
	canaryExporterParams := addCanaryExporterParams(canaryExporterCmd)

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
		if err := queryMerge(ctx, queryParams, *queryOutput); err != nil {
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
