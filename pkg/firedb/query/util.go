package parquetquery

import (
	"strings"

	pq "github.com/segmentio/parquet-go"
)

func GetColumnIndexByPath(pf *pq.File, s string) (index, depth int) {
	colSelector := strings.Split(s, ".")
	n := pf.Root()
	for len(colSelector) > 0 {
		n = n.Column(colSelector[0])
		if n == nil {
			return -1, -1
		}

		colSelector = colSelector[1:]
		depth++
	}

	return n.Index(), depth
}

func HasColumn(pf *pq.File, s string) bool {
	index, _ := GetColumnIndexByPath(pf, s)
	return index >= 0
}
