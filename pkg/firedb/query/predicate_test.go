package parquetquery

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

func TestSubstringPredicate(t *testing.T) {

	// Normal case - all chunks/pages/values inspected
	testPredicate(t, predicateTestCase{
		predicate:  NewSubstringPredicate("b"),
		keptChunks: 1,
		keptPages:  1,
		keptValues: 2,
		writeData: func(w *parquet.Writer) {
			type String struct {
				S string `parquet:",dict"`
			}
			require.NoError(t, w.Write(&String{"abc"})) // kept
			require.NoError(t, w.Write(&String{"bcd"})) // kept
			require.NoError(t, w.Write(&String{"cde"})) // skipped
		},
	})

	// Dictionary in the page header allows for skipping a page
	testPredicate(t, predicateTestCase{
		predicate:  NewSubstringPredicate("x"), // Not present in any values
		keptChunks: 1,
		keptPages:  0,
		keptValues: 0,
		writeData: func(w *parquet.Writer) {
			type dictString struct {
				S string `parquet:",dict"`
			}
			require.NoError(t, w.Write(&dictString{"abc"}))
			require.NoError(t, w.Write(&dictString{"bcd"}))
			require.NoError(t, w.Write(&dictString{"cde"}))
		},
	})
}

type predicateTestCase struct {
	writeData  func(w *parquet.Writer)
	keptChunks int
	keptPages  int
	keptValues int
	predicate  Predicate
}

// testPredicate by writing data and then iterating the column.  The data model
// must contain a single column.
func testPredicate(t *testing.T, tc predicateTestCase) {
	buf := new(bytes.Buffer)
	w := parquet.NewWriter(buf)
	tc.writeData(w)
	w.Flush()
	w.Close()

	file := bytes.NewReader(buf.Bytes())
	r, err := parquet.OpenFile(file, int64(buf.Len()))
	require.NoError(t, err)

	p := InstrumentedPredicate{pred: tc.predicate}

	i := NewColumnIterator(context.TODO(), r.RowGroups(), 0, "test", 100, &p, "")
	for i.Next() {
	}

	require.Equal(t, tc.keptChunks, int(p.KeptColumnChunks.Load()), "keptChunks")
	require.Equal(t, tc.keptPages, int(p.KeptPages.Load()), "keptPages")
	require.Equal(t, tc.keptValues, int(p.KeptValues.Load()), "keptValues")
}

func BenchmarkSubstringPredicate(b *testing.B) {
	p := NewSubstringPredicate("abc")

	s := make([]parquet.Value, 1000)
	for i := 0; i < 1000; i++ {
		s[i] = parquet.ValueOf(uuid.New().String())
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, ss := range s {
			p.KeepValue(ss)
		}
	}
}
