package tree

import (
	"fmt"
	"strings"
)

func (t *Tree) Collapsed() string {
	t.RLock()
	defer t.RUnlock()

	var res strings.Builder

	t.IterateStacks(func(_ string, self uint64, stack []string) {
		for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
			stack[i], stack[j] = stack[j], stack[i]
		}
		v2 := fmt.Sprintf("%s %d\n", strings.Join(stack, ";"), self)
		res.WriteString(v2)
	})

	return res.String()
}
