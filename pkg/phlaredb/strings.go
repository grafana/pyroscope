// nolint unused
package phlaredb

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
	var maxID int64
	for id := range m {
		if id > maxID {
			maxID = id
		}
	}
	r.strings = make(stringConversionTable, maxID+1)

	for x, y := range m {
		r.strings[x] = y
	}
}

// nolint unused
func (*stringsHelper) rewrite(*rewriter, string) error {
	return nil
}

func (*stringsHelper) size(s string) uint64 {
	return uint64(len(s))
}

func (*stringsHelper) setID(oldID, newID uint64, s string) uint64 {
	return oldID
}

func (*stringsHelper) clone(s string) string {
	return s
}
