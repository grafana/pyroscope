package querybackend

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/grafana/pyroscope/v2/pkg/block"
)

// The suite is run with the objects and datasets read from the storage
// in ranges, as large ones are, with and without read-ahead.
func TestSuite_ReadAhead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rgs      int
		maxBytes int64
		datasets int
	}{
		{name: "disabled"},
		{name: "row groups 1", rgs: 1, maxBytes: 32 << 20},
		{name: "row groups 4", rgs: 4, maxBytes: 32 << 20},
		{name: "row groups 2, chunks over limit", rgs: 2, maxBytes: 1 << 10},
		{name: "row groups 2, small limit", rgs: 2, maxBytes: 16 << 10},
		{name: "datasets 2", datasets: 2},
		{name: "datasets 2, row groups 2", rgs: 2, maxBytes: 32 << 20, datasets: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite.Run(t, &testSuite{
				dir: "testdata/samples",
				configure: func(r *BlockReader) {
					r.objectOptions = []block.ObjectOption{block.WithObjectMaxSizeLoadInMemory(0)}
					r.datasetOptions = []block.DatasetOption{block.WithDatasetMaxSizeLoadInMemory(0)}
					r.ReadAhead = block.ReadAheadOptions{RowGroups: tc.rgs, MaxBytes: tc.maxBytes}
					r.DatasetReadAhead = tc.datasets
				},
			})
		})
	}
}
