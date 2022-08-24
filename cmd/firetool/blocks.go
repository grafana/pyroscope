package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/thanos-io/objstore/providers/filesystem"

	"github.com/grafana/fire/pkg/firedb"
)

func tableInfo(info firedb.TableInfo) string {
	return fmt.Sprintf("%d (%s in %d RGs)", info.Rows, humanize.Bytes(info.Bytes), info.RowGroups)
}

func blocksList(_ context.Context) error {
	bucket, err := filesystem.NewBucket(cfg.blocks.path)
	if err != nil {
		return err
	}

	q := firedb.NewBlockQuerier(logger, bucket)
	if err := q.Open(); err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Block ID", "MinTime", "MaxTime", "Duration", "Profiles", "Stacktraces", "Locations", "Functions", "Strings"})
	for _, blockInfo := range q.BlockInfo() {
		table.Append([]string{
			blockInfo.ID.String(),
			blockInfo.MinTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Format(time.RFC3339),
			blockInfo.MaxTime.Time().Sub(blockInfo.MinTime.Time()).String(),
			tableInfo(blockInfo.Profiles),
			tableInfo(blockInfo.Stacktraces),
			tableInfo(blockInfo.Locations),
			tableInfo(blockInfo.Functions),
			tableInfo(blockInfo.Strings),
		})
	}
	table.Render()

	return nil
}
