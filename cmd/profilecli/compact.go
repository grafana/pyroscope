package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

func blocksCompact(ctx context.Context, src, dst string, shards int) error {
	var inputBlocks []*block.Meta
	_, ok := block.IsBlockDir(src)
	if ok {
		meta, err := block.ReadMetaFromDir(src)
		if err != nil {
			return err
		}
		src = filepath.Clean(filepath.Join(src, "/../"))
		inputBlocks = append(inputBlocks, meta)

	} else {
		blocks, err := block.ListBlocks(src, time.Time{})
		if err != nil {
			return err
		}
		if len(blocks) == 0 {
			return errors.New("no input blocks found, please provide either a single block directory or a directory containing blocks")
		}
		for _, b := range blocks {
			inputBlocks = append(inputBlocks, b.Clone())
		}
	}
	return compact(ctx, src, dst, inputBlocks, shards)
}

func compact(ctx context.Context, src, dst string, metas []*block.Meta, shards int) error {
	// create the destination directory if it doesn't exist
	if _, err := os.Stat(dst); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return errors.Wrap(err, "create dir")
		}
	}

	ctx = phlarecontext.WithLogger(ctx, log.NewNopLogger())
	blocks := make([]phlaredb.BlockReader, 0, len(metas))
	in := make([]block.Meta, 0, len(metas))

	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: src,
			},
		},
		StoragePrefix: "",
	}, "profilecli")
	if err != nil {
		return err
	}

	for _, m := range metas {
		in = append(in, *m)
		blocks = append(blocks, phlaredb.NewSingleBlockQuerierFromMeta(ctx, bkt, m))
	}
	fmt.Fprintln(output(ctx), "Found Input blocks:")
	printMeta(ctx, in)
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithWriter(output(ctx)))
	s.Suffix = " Loading data..."
	s.Start()
	g, groupCtx := errgroup.WithContext(ctx)

	for _, b := range blocks {
		b := b
		g.Go(func() error {
			if err := b.Open(groupCtx); err != nil {
				return err
			}
			return b.Symbols().Load(groupCtx)
		})
	}
	if err := g.Wait(); err != nil {
		s.Stop()
		return err
	}

	s.Suffix = " Compacting data..."
	s.Restart()

	out, err := phlaredb.CompactWithSplitting(ctx, phlaredb.CompactWithSplittingOpts{
		Src:                blocks,
		Dst:                dst,
		SplitCount:         uint64(shards),
		StageSize:          0,
		SplitBy:            phlaredb.SplitByFingerprint,
		DownsamplerEnabled: true,
		Logger:             logger,
	})
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Fprintln(output(ctx), "Output blocks:")
	printMeta(ctx, out)
	return nil
}

func printMeta(ctx context.Context, metas []block.Meta) {
	table := tablewriter.NewWriter(output(ctx))
	table.SetHeader([]string{"Block ID", "MinTime", "MaxTime", "Duration", "Index", "Profiles", "Symbols", "Labels"})
	for _, blockInfo := range metas {
		table.Append([]string{
			blockInfo.ULID.String(),
			blockInfo.MinTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Sub(blockInfo.MinTime.Time()).String(),
			indexInfo(blockInfo),
			fileInfo(blockInfo.FileByRelPath("profiles.parquet")),
			symbolSize(blockInfo),
			labelsString(blockInfo.Labels),
		})
	}
	table.Render()
}

func indexInfo(meta block.Meta) string {
	if f := meta.FileByRelPath("index.tsdb"); f != nil {
		return fmt.Sprintf("%d series (%s)", f.TSDB.NumSeries, humanize.Bytes(f.SizeBytes))
	}
	return ""
}

func symbolSize(meta block.Meta) string {
	size := uint64(0)
	if f := meta.FileByRelPath("symbols/index.symdb"); f != nil {
		size += f.SizeBytes
	}
	if f := meta.FileByRelPath("symbols/stacktraces.symdb"); f != nil {
		size += f.SizeBytes
	}
	if f := meta.FileByRelPath("symbols/locations.parquet"); f != nil {
		size += f.SizeBytes
	}
	if f := meta.FileByRelPath("symbols/functions.parquet"); f != nil {
		size += f.SizeBytes
	}
	if f := meta.FileByRelPath("symbols/mapping.parquet"); f != nil {
		size += f.SizeBytes
	}
	if f := meta.FileByRelPath("symbols/strings.parquet"); f != nil {
		size += f.SizeBytes
	}

	return humanize.Bytes(size)
}

func labelsString(m map[string]string) string {
	var s string
	for k, v := range m {
		s += fmt.Sprintf("%s=%s,", k, v)
	}
	return s
}
