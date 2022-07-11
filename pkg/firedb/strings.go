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
