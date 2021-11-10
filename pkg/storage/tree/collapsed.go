package tree

import (
	"fmt"
	"sort"
	"strings"
)

func (t *Tree) Collapsed() string {
	t.RLock()
	defer t.RUnlock()

	var res strings.Builder

	t.Iterate2(func(_ string, self uint64, stack []string) {
		sort.Sort(sort.Reverse(sort.StringSlice(stack)))
		v2 := fmt.Sprintf("%s %d\n", strings.Join(stack, ";"), self)
		res.WriteString(v2)
	})

	return res.String()
}
