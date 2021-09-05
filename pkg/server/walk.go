package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type treeNodeStats struct {
	key    string
	nodes  int
	leaves int
}

func (ctrl *Controller) walkHandler(w http.ResponseWriter, _ *http.Request) {
	var l []treeNodeStats
	walk := func(k string, v interface{}) {
		x, y := v.(*tree.Tree).Size()
		l = append(l, treeNodeStats{k, x, y})
	}
	ctrl.storage.WalkTrees(walk)
	sort.Slice(l, func(i, j int) bool {
		return l[i].key < l[j].key
	})
	for i := range l {
		_, _ = fmt.Fprintln(w, l[i].key, l[i].nodes, l[i].leaves)
	}
}
