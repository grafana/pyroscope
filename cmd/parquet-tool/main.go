package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/segmentio/parquet-go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: parquet-tool <file>")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	stats, err := f.Stat()
	if err != nil {
		panic(err)
	}
	pf, err := parquet.OpenFile(f, stats.Size())
	if err != nil {
		panic(err)
	}
	fmt.Println("schema:", pf.Schema())
	meta := pf.Metadata()
	fmt.Println("Num Rows:", meta.NumRows)
	for i, rg := range meta.RowGroups {
		fmt.Println("\t Row group:", i)
		fmt.Println("\t\t Row Count:", rg.NumRows)
		fmt.Println("\t\t Row size:", humanize.Bytes(uint64(rg.TotalByteSize)))
		fmt.Println("\t\t Columns:")
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{
			"Col", "Type", "NumVal", "TotalCompressedSize", "TotalUncompressedSize", "Compression", "%", "PageCount", "AvgPageSize",
		})
		for j, ds := range rg.Columns {
			offsets := pf.OffsetIndexes()[j]
			var avgPageSize int64
			for _, offset := range offsets.PageLocations {
				avgPageSize += int64(offset.CompressedPageSize)
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
					humanize.Bytes(uint64(avgPageSize)),
				})
		}
		table.Render()
	}
}
