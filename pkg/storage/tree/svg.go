package tree

import (
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/petethepig/pyroscope/pkg/structs/cappedarr"
	"github.com/petethepig/pyroscope/pkg/svg"
)

func (tn2 *treeNode) svg2(w io.Writer, maxDepth, minVal uint64, totalCum float64, width int) {
	nodes := []*treeNode{tn2}
	xOffsets := []float64{0.0}
	levels := []uint64{0}
	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		xOffset := xOffsets[0]
		xOffsets = xOffsets[1:]

		l := levels[0]
		levels = levels[1:]

		wk := float64(tn.cum) / totalCum
		w2 := (float64(width) - svg.Margin*2)
		wwk := wk * w2
		if tn.cum > minVal {
			label := tn.name
			svg.RenderBlock(w, label, maxDepth-l, tn.cum, wwk, xOffset+svg.Margin, wk*100, float64(tn.self)/totalCum, len(tn.childrenNodes))

			xOffset += float64(tn.self) / totalCum * w2
			childrenCum := uint64(0)
			for _, n := range tn.childrenNodes {
				if n.cum > minVal {
					// n.svg(w, l+1, maxDepth, minVal, totalCum, xOffset)
					xOffsets = append([]float64{xOffset}, xOffsets...)
					levels = append([]uint64{l + 1}, levels...)
					nodes = append([]*treeNode{n}, nodes...)
					xOffset += float64(n.cum) / totalCum * w2
				} else {
					childrenCum += n.cum
				}
			}
			// TODO: add other node
			if childrenCum > 0 {
				// xOffsets = append([]float64{xOffset}, xOffsets...)
				// levels = append([]uint64{l + 1}, levels...)
				// otherNode := &treeNode{
				// 	labelLink:     nil,
				// 	cum:           childrenCum,
				// 	self:          childrenCum,
				// 	childrenNodes: nil,
				// }
				// nodes = append([]*treeNode{otherNode}, nodes...)
			}
		}
	}
}

func (n *treeNode) maxDepth(startDepth int, minSamples uint64) int {
	max := startDepth
	if n.cum >= minSamples {
		for _, child := range n.childrenNodes {
			d := child.maxDepth(startDepth+1, minSamples)
			if d > max {
				max = d
			}
		}
	}

	return max
}

func (t *Tree) minValue(maxNodes int) uint64 {
	c := cappedarr.New(maxNodes)
	t.IterateWithCum(func(cum uint64) bool {
		return c.Push(cum)
	})
	return c.MinValue()
}

func (t *Tree) SVG(w io.Writer, maxNodes uint64, width int) {
	minSamples := t.minValue(int(maxNodes))

	if t.root != nil {
		maxDepth := t.root.maxDepth(0, minSamples)
		h := svg.Header{
			Width:  width,
			TitleX: width / 2,
			Height: maxDepth*int(svg.Hd+1) + 70,
			LabelY: maxDepth*int(svg.Hd+1) + 101 - 48,
		}
		log.Debug("SVG maxDepth", maxDepth)
		svg.HeaderTmplt.Execute(w, h)
		t.root.svg2(w, uint64(maxDepth), minSamples, float64(t.root.cum), width)
	}
	w.Write([]byte(svg.FooterStr))
}
