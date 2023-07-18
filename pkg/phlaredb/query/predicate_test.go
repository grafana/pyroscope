package query

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

type dictString struct {
	S string `parquet:",dict"`
}

type String struct {
	S string `parquet:",dict"`
}

func TestSubstringPredicate(t *testing.T) {
	// Normal case - all chunks/pages/values inspected
	testPredicate(t, predicateTestCase[String]{
		predicate:  NewSubstringPredicate("b"),
		keptChunks: 1,
		keptPages:  1,
		keptValues: 2,
		writeData: func(w *parquet.GenericWriter[String]) {
			_, err := w.Write([]String{{"abc"}})
			require.NoError(t, err) // kept
			_, err = w.Write([]String{{"bcd"}})
			require.NoError(t, err) // kept
			_, err = w.Write([]String{{"cde"}})
			require.NoError(t, err) // skipped
		},
	})

	// Dictionary in the page header allows for skipping a page
	testPredicate(t, predicateTestCase[dictString]{
		predicate:  NewSubstringPredicate("x"), // Not present in any values
		keptChunks: 1,
		keptPages:  0,
		keptValues: 0,
		writeData: func(w *parquet.GenericWriter[dictString]) {
			_, err := w.Write([]dictString{{"abc"}})
			require.NoError(t, err)
			_, err = w.Write([]dictString{{"bcd"}})
			require.NoError(t, err)
			_, err = w.Write([]dictString{{"cde"}})
			require.NoError(t, err)
		},
	})
}

type predicateTestCase[P any] struct {
	writeData  func(w *parquet.GenericWriter[P])
	keptChunks int
	keptPages  int
	keptValues int
	predicate  Predicate
}

// testPredicate by writing data and then iterating the column.  The data model
// must contain a single column.
func testPredicate[T any](t *testing.T, tc predicateTestCase[T]) {
	buf := new(bytes.Buffer)
	w := parquet.NewGenericWriter[T](buf)
	tc.writeData(w)
	w.Flush()
	w.Close()

	file := bytes.NewReader(buf.Bytes())
	r, err := parquet.OpenFile(file, int64(buf.Len()))
	require.NoError(t, err)

	p := InstrumentedPredicate{pred: tc.predicate}

	i := NewSyncIterator(context.TODO(), r.RowGroups(), 0, "test", 100, &p, "")
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
