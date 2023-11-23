package query

import (
	"strings"

	"github.com/colega/zeropool"
	"github.com/parquet-go/parquet-go"
)

func GetColumnIndexByPath(root *parquet.Column, s string) (index, depth int) {
	colSelector := strings.Split(s, ".")
	n := root
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

func HasColumn(root *parquet.Column, s string) bool {
	index, _ := GetColumnIndexByPath(root, s)
	return index >= 0
}

func RowGroupBoundaries(groups []parquet.RowGroup) []int64 {
	b := make([]int64, len(groups))
	var o int64
	for i := range b {
		o += groups[i].NumRows()
		b[i] = o
	}
	return b
}

func SplitRows(rows, groups []int64) [][]int64 {
	switch len(groups) {
	case 0:
		return nil
	case 1:
		return [][]int64{rows}
	}
	// Sanity check: max row must be less than
	// the number of rows in the last group.
	if rows[len(rows)-1] >= groups[len(groups)-1] {
		panic(ErrSeekOutOfRange)
	}
	split := make([][]int64, len(groups))
	var j, r int
	maxRow := groups[j]
	for i, rn := range rows {
		if rn < maxRow {
			continue
		}
		split[j], rows = rows[:i-r], rows[i-r:]
		r = i
		// Find matching group.
		for x, v := range groups[j:] {
			if rn >= v {
				continue
			}
			j += x
			break
		}
		maxRow = groups[j]
	}
	// Last bit.
	split[j] = rows
	// Subtract group offset from the row numbers,
	// which makes them local to the group.
	for i, g := range split[1:] {
		offset := groups[i]
		for n := range g {
			g[n] -= offset
		}
	}
	return split
}

var parquetValuesPool = zeropool.New(func() []parquet.Value { return nil })

func CloneParquetValues(values []parquet.Value) []parquet.Value {
	p := parquetValuesPool.Get()
	if l := len(values); cap(p) < l {
		p = make([]parquet.Value, 0, 2*l)
	}
	p = p[:len(values)]
	for i, v := range values {
		p[i] = v.Clone()
	}
	return p
}

func ReleaseParquetValues(b [][]parquet.Value) {
	for _, s := range b {
		if cap(s) > 0 {
			parquetValuesPool.Put(s)
		}
	}
}

var uint64valuesPool = zeropool.New(func() []uint64 { return nil })

func CloneUint64ParquetValues(values []parquet.Value) []uint64 {
	uint64s := uint64valuesPool.Get()
	if l := len(values); cap(uint64s) < l {
		uint64s = make([]uint64, 0, 2*l)
	}
	uint64s = uint64s[:len(values)]
	for i, v := range values {
		uint64s[i] = v.Uint64()
	}
	return uint64s
}

func ReleaseUint64Values(b [][]uint64) {
	for _, s := range b {
		if len(s) > 0 {
			uint64valuesPool.Put(s)
		}
	}
}

var uint32valuesPool = zeropool.New(func() []uint32 { return nil })

func CloneUint32ParquetValues(values []parquet.Value) []uint32 {
	uint32s := uint32valuesPool.Get()
	if l := len(values); cap(uint32s) < l {
		uint32s = make([]uint32, 0, 2*l)
	}
	uint32s = uint32s[:len(values)]
	for i, v := range values {
		uint32s[i] = v.Uint32()
	}
	return uint32s
}

func ReleaseUint32Values(b [][]uint32) {
	for _, s := range b {
		if len(s) > 0 {
			uint32valuesPool.Put(s)
		}
	}
}
