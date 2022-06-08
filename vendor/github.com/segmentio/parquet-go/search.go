package parquet

// CompareNullsFirst constructs a comparison function which assumes that null
// values are smaller than all other values.
func CompareNullsFirst(cmp func(Value, Value) int) func(Value, Value) int {
	return func(a, b Value) int {
		switch {
		case a.IsNull():
			if b.IsNull() {
				return 0
			}
			return -1
		case b.IsNull():
			return +1
		default:
			return cmp(a, b)
		}
	}
}

// CompareNullsLast constructs a comparison function which assumes that null
// values are greater than all other values.
func CompareNullsLast(cmp func(Value, Value) int) func(Value, Value) int {
	return func(a, b Value) int {
		switch {
		case a.IsNull():
			if b.IsNull() {
				return 0
			}
			return +1
		case b.IsNull():
			return -1
		default:
			return cmp(a, b)
		}
	}
}

// Search is like Find, but uses the default ordering of the given type.
func Search(index ColumnIndex, value Value, typ Type) int {
	return Find(index, value, CompareNullsLast(typ.Compare))
}

// Find uses the column index passed as argument to find the page that the
// given value is expected to be found in.
//
// The function returns the index of the first page that might contain the
// value. If the function determines that the value does not exist in the
// index, NumPages is returned.
//
// The comparison function passed as last argument is used to determine the
// relative order of values. This should generally be the Compare method of
// the column type, but can sometimes be customized to modify how null values
// are interpreted, for example:
//
//	pageIndex := parquet.Find(columnIndex, value,
//		parquet.CompareNullsFirst(typ.Compare),
//	)
//
func Find(index ColumnIndex, value Value, cmp func(Value, Value) int) int {
	switch {
	case index.IsAscending():
		return binarySearch(index, value, cmp)
	default:
		return linearSearch(index, value, cmp)
	}
}

func binarySearch(index ColumnIndex, value Value, cmp func(Value, Value) int) int {
	n := index.NumPages()
	i := 0
	j := n

	for (j - i) > 1 {
		k := ((j - i) / 2) + i
		c := cmp(value, index.MinValue(k))

		switch {
		case c < 0:
			j = k
		case c > 0:
			i = k
		default:
			return k
		}
	}

	if i < n {
		min := index.MinValue(i)
		max := index.MaxValue(i)

		if cmp(value, min) < 0 || cmp(max, value) < 0 {
			i = n
		}
	}

	return i
}

func linearSearch(index ColumnIndex, value Value, cmp func(Value, Value) int) int {
	n := index.NumPages()

	for i := 0; i < n; i++ {
		min := index.MinValue(i)
		max := index.MaxValue(i)

		if cmp(min, value) <= 0 && cmp(value, max) <= 0 {
			return i
		}
	}

	return n
}
