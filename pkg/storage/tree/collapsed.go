package tree

import (
	"fmt"
	"strings"
)

func (t *Tree) Collapsed() string {
	t.RLock()
	defer t.RUnlock()

	var res strings.Builder

	t.Iterate(func(k []byte, v uint64) {
		if v > 0 {
			v2 := fmt.Sprintf("%q %d\n", k[2:], v)
			res.WriteString(v2)
		}
	})

	return res.String()
}
