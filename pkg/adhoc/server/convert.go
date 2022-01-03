package server

import (
	"fmt"
	"sort"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

func profileToTree(fb flamebearer.FlamebearerProfile) (*tree.Tree, error) {
	if fb.Metadata.Format != string(tree.FormatSingle) {
		return nil, fmt.Errorf("unsupported flamebearer format %s", fb.Metadata.Format)
	}
	return flamebearerV1ToTree(fb.Flamebearer)
}

func flamebearerV1ToTree(fb flamebearer.FlamebearerV1) (*tree.Tree, error) {
	t := tree.New()
	deltaDecoding(fb.Levels, 0, 4)
	for i, l := range fb.Levels {
		for j := 0; j < len(l); j += 4 {
			self := l[j+2]
			if self > 0 {
				t.InsertStackString(buildStack(fb, i, j), uint64(self))
			}
		}
	}
	return t, nil
}

func deltaDecoding(levels [][]int, start, step int) {
	for _, l := range levels {
		prev := 0
		for i := start; i < len(l); i += step {
			delta := l[i] + l[i+1]
			l[i] += prev
			prev += delta
		}
	}
}

func buildStack(fb flamebearer.FlamebearerV1, level, idx int) []string {
	stack := make([]string, level+1)
	stack[level] = fb.Names[fb.Levels[level][idx+3]]
	x := fb.Levels[level][idx]
	for i := level - 1; i >= 0; i-- {
		j := sort.Search(len(fb.Levels[i])/4, func(j int) bool { return fb.Levels[i][j*4] > x }) - 1
		stack[i] = fb.Names[fb.Levels[i][j*4+3]]
		x = fb.Levels[i][j*4]
	}
	return stack
}
