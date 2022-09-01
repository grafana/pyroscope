package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"
	"github.com/olekukonko/tablewriter"

	"github.com/grafana/fire/pkg/firedb"
	"github.com/grafana/fire/pkg/firedb/block"
	"github.com/grafana/fire/pkg/objstore/providers/filesystem"
)

func fileInfo(f *block.File) string {
	if f != nil && f.Parquet != nil {
		return fmt.Sprintf("%d (%s in %d RGs)", f.Parquet.NumRows, humanize.Bytes(f.SizeBytes), f.Parquet.NumRowGroups)
	}
	if f != nil && f.TSDB != nil {
		return fmt.Sprintf("%d series", f.TSDB.NumSeries)
	}
	return ""
}

func blocksList(ctx context.Context) error {
	bucket, err := filesystem.NewBucket(cfg.blocks.path)
	if err != nil {
		return err
	}

	metas, err := firedb.NewBlockQuerier(logger, bucket).BlockMetas(ctx)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Block ID", "MinTime", "MaxTime", "Duration", "Index", "Profiles", "Stacktraces", "Locations", "Functions", "Strings"})
	for _, blockInfo := range metas {
		table.Append([]string{
			blockInfo.ULID.String(),
			blockInfo.MinTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Sub(blockInfo.MinTime.Time()).String(),
			fileInfo(blockInfo.FileByRelPath("index.tsdb")),
			fileInfo(blockInfo.FileByRelPath("profiles.parquet")),
			fileInfo(blockInfo.FileByRelPath("stacktraces.parquet")),
			fileInfo(blockInfo.FileByRelPath("locations.parquet")),
			fileInfo(blockInfo.FileByRelPath("functions.parquet")),
			fileInfo(blockInfo.FileByRelPath("strings.parquet")),
		})

		if cfg.blocks.restoreMissingMeta {
			blockPath := filepath.Join(cfg.blocks.path, blockInfo.ULID.String())
			blockMetaPath := filepath.Join(blockPath, block.MetaFilename)

			if _, err := os.Stat(blockMetaPath); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				level.Error(logger).Log("msg", "unable to check for existing "+block.MetaFilename+" file", "path", blockMetaPath, "err", err)
				continue
			}

			if _, err := blockInfo.WriteToFile(logger, blockPath); err != nil {
				level.Error(logger).Log("msg", "unable to write regenerated "+block.MetaFilename, "path", blockMetaPath, "err", err)
				continue
			}
		}
	}
	table.Render()

	return nil
}
