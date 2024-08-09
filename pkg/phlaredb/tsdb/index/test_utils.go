package index

import (
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type IndexWriterSeries struct {
	Labels phlaremodel.Labels
	Chunks []ChunkMeta // series file offset of chunks
}

type IndexWriterSeriesSlice []*IndexWriterSeries

func (s IndexWriterSeriesSlice) Len() int      { return len(s) }
func (s IndexWriterSeriesSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s IndexWriterSeriesSlice) Less(i, j int) bool {
	return phlaremodel.CompareLabelPairs(s[i].Labels, s[j].Labels) < 0
}
