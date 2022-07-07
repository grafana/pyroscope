package index

import (
	firemodel "github.com/grafana/fire/pkg/model"
)

type indexWriterSeries struct {
	labels firemodel.Labels
	chunks []ChunkMeta // series file offset of chunks
}

type indexWriterSeriesSlice []*indexWriterSeries

func (s indexWriterSeriesSlice) Len() int      { return len(s) }
func (s indexWriterSeriesSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s indexWriterSeriesSlice) Less(i, j int) bool {
	return firemodel.CompareLabelPairs(s[i].labels, s[j].labels) < 0
}
