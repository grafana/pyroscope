package main

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Input struct {
	Timeline    *segment.Timeline `json:"timeline"`
	Flamebearer *tree.Flamebearer `json:"flamebearer"`
	Metadata    *InputMetadata    `json:"metadata"`
}

type InputMetadata struct {
	SpyName    string      `json:"spyName"`
	SampleRate uint32      `json:"sampleRate"`
	Units      string      `json:"units"`
	Format     tree.Format `json:"format"`
}

type Output struct {
	Flamebearer *OutputFlamebearer `json:"flamebearer"`
}

type OutputFlamebearer struct {
	Levels [][]OutputItem `json:"levels"`
}

type OutputItem interface{}

type OutputItemSingle struct {
	Row    int    `json:"_row"`
	Col    int    `json:"_col"`
	Name   string `json:"name"`
	Total  int    `json:"total"`
	Self   int    `json:"self"`
	Offset int    `json:"offset"`
}

type OutputItemDouble struct {
	Row        int    `json:"_row"`
	Col        int    `json:"_col"`
	Name       string `json:"name"`
	LeftSelf   int    `json:"left_self"`
	LeftTotal  int    `json:"left_total"`
	LeftOffset int    `json:"left_offset"`
	RghtSelf   int    `json:"right_self"`
	RghtTotal  int    `json:"right_total"`
	RghtOffset int    `json:"right_offset"`
}

func decodeLevels(in *Input) *Output {
	names, levels := in.Flamebearer.Names, in.Flamebearer.Levels
	outLevels := make([][]OutputItem, 0, len(levels))
	isSingle := in.Flamebearer.Format != tree.FormatDouble

	step := 4
	if !isSingle {
		step = 7
	}

	for rowIdx, row := range levels {
		offsetLeft, offsetRght := 0, 0
		outRow := make([]OutputItem, 0, len(row))

		for i, N := 0, len(row); i < N; i += step {
			var outItem OutputItem
			if isSingle {
				offsetLeft += row[i+0]
				outItem = OutputItemSingle{
					Row: rowIdx, Col: i / step,
					Offset: offsetLeft,
					Total:  row[i+1],
					Self:   row[i+2],
					Name:   names[row[i+3]],
				}

			} else {
				offsetLeft += row[i+0]
				offsetRght += row[i+3]
				outItem = OutputItemDouble{
					Row: rowIdx, Col: i / step,
					LeftOffset: offsetLeft,
					LeftTotal:  row[i+1],
					LeftSelf:   row[i+2],
					RghtOffset: offsetRght,
					RghtTotal:  row[i+4],
					RghtSelf:   row[i+6],
					Name:       names[row[i+6]],
				}
			}
			outRow = append(outRow, outItem)
		}
		outLevels = append(outLevels, outRow)
	}

	out := &Output{
		Flamebearer: &OutputFlamebearer{
			Levels: outLevels,
		},
	}
	return out
}
