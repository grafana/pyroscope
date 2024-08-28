package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/parquet-go/parquet-go"
)

func parquetInspect(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stats, err := f.Stat()
	if err != nil {
		return err
	}
	pf, err := parquet.OpenFile(f, stats.Size())
	if err != nil {
		return err
	}
	out := output(ctx)
	fmt.Fprintln(out, "schema:", pf.Schema())
	numColumns := len(pf.Schema().Columns())
	meta := pf.Metadata()
	fmt.Println("Num Rows:", meta.NumRows)
	for i, rg := range meta.RowGroups {
		fmt.Fprintln(out, "\t Row group:", i)
		fmt.Fprintln(out, "\t\t Row Count:", rg.NumRows)
		fmt.Fprintln(out, "\t\t Row size:", humanize.Bytes(uint64(rg.TotalByteSize)))
		fmt.Fprintln(out, "\t\t Columns:")
		table := tablewriter.NewWriter(out)
		table.SetHeader([]string{
			"Col", "Type", "NumVal", "TotalCompressedSize", "TotalUncompressedSize", "Compression", "%", "PageCount", "PageSize",
		})

		for j, ds := range rg.Columns {
			offsets := pf.OffsetIndexes()[(i*numColumns)+j]
			var avgPageSize int64
			maxPageSize := int64(0)
			minPageSize := int64(math.MaxInt64)
			for _, offset := range offsets.PageLocations {
				avgPageSize += int64(offset.CompressedPageSize)
				if int64(offset.CompressedPageSize) > maxPageSize {
					maxPageSize = int64(offset.CompressedPageSize)
				}
				if int64(offset.CompressedPageSize) < minPageSize {
					minPageSize = int64(offset.CompressedPageSize)
				}
			}
			avgPageSize /= int64(len(offsets.PageLocations))

			table.Append(
				[]string{
					strings.Join(ds.MetaData.PathInSchema, "/"),
					ds.MetaData.Type.String(),
					fmt.Sprintf("%d", ds.MetaData.NumValues),
					humanize.Bytes(uint64(ds.MetaData.TotalCompressedSize)),
					humanize.Bytes(uint64(ds.MetaData.TotalUncompressedSize)),
					fmt.Sprintf("%.2f", float64(ds.MetaData.TotalUncompressedSize-ds.MetaData.TotalCompressedSize)/float64(ds.MetaData.TotalCompressedSize)*100),
					fmt.Sprintf("%.2f", float64(ds.MetaData.TotalCompressedSize)/float64(rg.TotalByteSize)*100),
					fmt.Sprintf("%d", len(offsets.PageLocations)),
					"avg:" + humanize.Bytes(uint64(avgPageSize)) + ", max:" + humanize.Bytes(uint64(maxPageSize)) + ", min:" + humanize.Bytes(uint64(minPageSize)),
				})
		}
		table.Render()
	}

	return nil
}
