package firedb

type stringConversionTable []int64

func (t stringConversionTable) rewrite(idx *int64) {
	originalValue := int(*idx)
	newValue := t[originalValue]
	*idx = newValue
}

type stringsHelper struct{}

func (*stringsHelper) key(s string) string {
	return s
}

func (*stringsHelper) addToRewriter(r *rewriter, m idConversionTable) {
	r.strings = make(stringConversionTable, len(m))
	for x, y := range m {
		r.strings[x] = y
	}
}

func (*stringsHelper) rewrite(*rewriter, string) error {
	return nil
}

type stringRow struct {
	ID     uint64 `parquet:",delta"`
	String string `parquet:",dict"`
}

func stringSliceToRows(strs []string) []stringRow {
	rows := make([]stringRow, len(strs))
	for pos := range strs {
		rows[pos].ID = uint64(pos)
		rows[pos].String = strs[pos]
	}

	return rows
}
