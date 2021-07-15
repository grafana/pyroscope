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
	SpyName    string `json:"spyName"`
	SampleRate uint32 `json:"sampleRate"`
	Units      string `json:"units"`
}

type Output struct {
	Flamebearer *OutputFlamebearer `json:"flamebearer"`
}

type OutputFlamebearer struct {
	Levels [][]OutputItem `json:"levels"`
}

type OutputItem struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
	Self  int    `json:"self"`
}

func decodeLevels(in *Input) *Output {
	names, levels := in.Flamebearer.Names, in.Flamebearer.Levels
	outLevels := make([][]OutputItem, 0, len(levels))

	for _, row := range levels {
		offset := 0
		outRow := make([]OutputItem, 0, len(row))

		for i, N := 0, len(row); i < N; i += 4 {
			offset += row[i]
			outItem := OutputItem{
				Name:  names[row[i+3]],
				Total: offset + row[i+1],
				Self:  row[i+2],
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
