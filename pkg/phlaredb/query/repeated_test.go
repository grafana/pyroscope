package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/iter"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

type repeatedTestRow struct {
	List []int64
}

type testRowGetter struct {
	RowNum int64
}

func (t testRowGetter) RowNumber() int64 {
	return t.RowNum
}

func Test_RepeatedRowIterator_SingleColumn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     []testRowGetter
		rgs      [][]repeatedTestRow
		expected []RepeatedRow[testRowGetter]
		readSize int
	}{
		{
			name: "single row group no repeated and repeated",
			rows: []testRowGetter{
				{0},
				{1},
				{2},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 1, 1, 1}},
					{[]int64{2}},
					{[]int64{3, 4}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(1), parquet.ValueOf(1), parquet.ValueOf(1)}}},
				{testRowGetter{1}, [][]parquet.Value{{parquet.ValueOf(2)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(3), parquet.ValueOf(4)}}},
			},
		},
		{
			name: "multiple row group no repeated skip group and page",
			rows: []testRowGetter{
				{0},
				{2},
				{7},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1}},
					{[]int64{2}},
					{[]int64{3}},
				},
				{
					{[]int64{4}},
					{[]int64{5}},
					{[]int64{6}},
				},
				{
					{[]int64{7}},
					{[]int64{8}},
					{[]int64{9}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(3)}}},
				{testRowGetter{7}, [][]parquet.Value{{parquet.ValueOf(8)}}},
			},
		},
		{
			name: "single row group",
			rows: []testRowGetter{
				{0},
				{1},
				{2},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}},
					{[]int64{4, 5, 6}},
					{[]int64{7, 8, 9}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}}},
				{testRowGetter{1}, [][]parquet.Value{{parquet.ValueOf(4), parquet.ValueOf(5), parquet.ValueOf(6)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(7), parquet.ValueOf(8), parquet.ValueOf(9)}}},
			},
		},
		{
			name: "skip row group",
			rows: []testRowGetter{
				{0}, {1}, {2}, {6}, {7}, {8},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}},
					{[]int64{4, 5, 6}},
					{[]int64{7, 8, 9}},
				},
				{
					{[]int64{10, 11, 12}},
					{[]int64{13, 14, 15}},
					{[]int64{16, 17, 18}},
				},
				{
					{[]int64{19, 20, 21}},
					{[]int64{22, 23, 24}},
					{[]int64{25, 26, 27}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}}},
				{testRowGetter{1}, [][]parquet.Value{{parquet.ValueOf(4), parquet.ValueOf(5), parquet.ValueOf(6)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(7), parquet.ValueOf(8), parquet.ValueOf(9)}}},
				{testRowGetter{6}, [][]parquet.Value{{parquet.ValueOf(19), parquet.ValueOf(20), parquet.ValueOf(21)}}},
				{testRowGetter{7}, [][]parquet.Value{{parquet.ValueOf(22), parquet.ValueOf(23), parquet.ValueOf(24)}}},
				{testRowGetter{8}, [][]parquet.Value{{parquet.ValueOf(25), parquet.ValueOf(26), parquet.ValueOf(27)}}},
			},
		},
		{
			name: "single row group skip through page",
			rows: []testRowGetter{
				{1},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}},
					{[]int64{4, 5, 6}},
					{[]int64{7, 8, 9}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{1}, [][]parquet.Value{{parquet.ValueOf(4), parquet.ValueOf(5), parquet.ValueOf(6)}}},
			},
		},
		{
			name: "multiple row group skip within page",
			rows: []testRowGetter{
				{0},
				{2},
				{5},
				{7},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}}, // 0
					{[]int64{4, 5, 6}},
					{[]int64{7, 8, 9}}, // 2
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
				},
				{
					{[]int64{10, 11, 12}}, // 5
					{[]int64{0, 0, 0}},
					{[]int64{13, 14, 15}}, // 7

				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(7), parquet.ValueOf(8), parquet.ValueOf(9)}}},
				{testRowGetter{5}, [][]parquet.Value{{parquet.ValueOf(10), parquet.ValueOf(11), parquet.ValueOf(12)}}},
				{testRowGetter{7}, [][]parquet.Value{{parquet.ValueOf(13), parquet.ValueOf(14), parquet.ValueOf(15)}}},
			},
		},
		{
			name: "multiple row group skip within and through pages and row group",
			rows: []testRowGetter{
				{0},
				{2},
				{8},
				{10},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}}, // 0
					{[]int64{4, 5, 6}},
					{[]int64{7, 8, 9}}, // 2
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
				},
				{
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
				},
				{
					{[]int64{10, 11, 12}}, // 8
					{[]int64{0, 0, 0}},
					{[]int64{13, 14, 15}}, // 10

				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(7), parquet.ValueOf(8), parquet.ValueOf(9)}}},
				{testRowGetter{8}, [][]parquet.Value{{parquet.ValueOf(10), parquet.ValueOf(11), parquet.ValueOf(12)}}},
				{testRowGetter{10}, [][]parquet.Value{{parquet.ValueOf(13), parquet.ValueOf(14), parquet.ValueOf(15)}}},
			},
		},
		{
			name: "multiple row group skip within and through pages and row group mix repeated",
			rows: []testRowGetter{
				{0},
				{2},
				{8},
				{10},
			},
			rgs: [][]repeatedTestRow{
				{
					{[]int64{1, 2, 3}}, // 0
					{[]int64{4, 5}},
					{[]int64{7}}, // 2
					{[]int64{0}},
					{[]int64{0, 0, 0}},
				},
				{
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
					{[]int64{0, 0, 0}},
				},
				{
					{[]int64{10, 11, 12}}, // 8
					{[]int64{0, 0, 0}},
					{[]int64{13, 14}}, // 10

				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, [][]parquet.Value{{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}}},
				{testRowGetter{2}, [][]parquet.Value{{parquet.ValueOf(7)}}},
				{testRowGetter{8}, [][]parquet.Value{{parquet.ValueOf(10), parquet.ValueOf(11), parquet.ValueOf(12)}}},
				{testRowGetter{10}, [][]parquet.Value{{parquet.ValueOf(13), parquet.ValueOf(14)}}},
			},
		},
	} {
		tc := tc
		for _, readSize := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 10000} {
			tc.readSize = readSize
			t.Run(tc.name+fmt.Sprintf("_rs_%d", readSize), func(t *testing.T) {
				var groups []parquet.RowGroup
				for _, rg := range tc.rgs {
					buffer := parquet.NewBuffer()
					for _, row := range rg {
						require.NoError(t, buffer.Write(row))
					}
					groups = append(groups, buffer)
				}
				actual := readRepeatedRowIterator(t,
					NewRepeatedRowIterator(context.Background(),
						iter.NewSliceIterator(tc.rows), groups, 0))
				if diff := cmp.Diff(tc.expected, actual, int64ParquetComparer()); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
			})
		}

	}
}

func Test_RepeatedRowIterator_Cancellation(t *testing.T) {
	var groups []parquet.RowGroup
	for _, rg := range [][]repeatedTestRow{
		{
			{[]int64{1, 1, 1, 1}},
			{[]int64{2}},
			{[]int64{3, 4}},
		},
	} {
		buffer := parquet.NewBuffer()
		for _, row := range rg {
			require.NoError(t, buffer.Write(row))
		}
		groups = append(groups, buffer)
	}

	rows := iter.NewSliceIterator([]testRowGetter{{0}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	it := NewRepeatedRowIterator(ctx, rows, groups, 0)
	assert.False(t, it.Next())
	assert.Error(t, context.Canceled, it.Err())
	assert.NoError(t, it.Close())
}

type multiColumnItem struct {
	X int64
	Y int64
}

type multiColumnRepeatedTestRow struct {
	List []multiColumnItem
}

func Test_RepeatedRowPageIterator_MultipleColumns(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     []testRowGetter
		rgs      [][]multiColumnRepeatedTestRow
		expected []RepeatedRow[testRowGetter]
	}{
		{
			name: "single row group",
			rows: []testRowGetter{
				{0},
			},
			rgs: [][]multiColumnRepeatedTestRow{
				{
					{
						List: []multiColumnItem{
							{1, 2},
							{3, 4},
							{5, 6},
						},
					},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{
					testRowGetter{0},
					[][]parquet.Value{
						{parquet.ValueOf(1), parquet.ValueOf(3), parquet.ValueOf(5)},
						{parquet.ValueOf(2), parquet.ValueOf(4), parquet.ValueOf(6)},
					},
				},
			},
		},
		{
			name: "row group and page seek",
			rows: []testRowGetter{
				{1},
				{4},
				{7},
			},
			rgs: [][]multiColumnRepeatedTestRow{
				{
					{List: []multiColumnItem{{0, 0}, {0, 0}}},
					{List: []multiColumnItem{{1, 2}, {3, 4}}}, // 1
					{List: []multiColumnItem{{0, 0}, {0, 0}}},
				},
				{
					{List: []multiColumnItem{{0, 0}, {0, 0}}},
					{List: []multiColumnItem{{5, 6}, {7, 8}}}, // 4
					{List: []multiColumnItem{{0, 0}, {0, 0}}},
					{List: []multiColumnItem{{0, 0}, {0, 0}}},
					{List: []multiColumnItem{{9, 10}}}, // 7
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{
					testRowGetter{1},
					[][]parquet.Value{
						{parquet.ValueOf(1), parquet.ValueOf(3)},
						{parquet.ValueOf(2), parquet.ValueOf(4)},
					},
				},
				{
					testRowGetter{4},
					[][]parquet.Value{
						{parquet.ValueOf(5), parquet.ValueOf(7)},
						{parquet.ValueOf(6), parquet.ValueOf(8)},
					},
				},
				{
					testRowGetter{7},
					[][]parquet.Value{
						{parquet.ValueOf(9)},
						{parquet.ValueOf(10)},
					},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var groups []parquet.RowGroup
			for _, rg := range tc.rgs {
				buffer := parquet.NewBuffer()
				for _, row := range rg {
					require.NoError(t, buffer.Write(row))
				}
				groups = append(groups, buffer)
			}
			actual := readRepeatedRowIterator(t,
				NewRepeatedRowIterator(context.Background(),
					iter.NewSliceIterator(tc.rows), groups, 0, 1),
			)
			if diff := cmp.Diff(tc.expected, actual, int64ParquetComparer()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func readRepeatedRowIterator(t *testing.T, it iter.Iterator[RepeatedRow[testRowGetter]]) []RepeatedRow[testRowGetter] {
	defer func() {
		require.NoError(t, it.Close())
	}()
	var result []RepeatedRow[testRowGetter]
	for it.Next() {
		current := RepeatedRow[testRowGetter]{
			Row:    it.At().Row,
			Values: make([][]parquet.Value, len(it.At().Values)),
		}
		for i, v := range it.At().Values {
			current.Values[i] = make([]parquet.Value, len(v))
			copy(current.Values[i], v)
		}
		if len(result) > 0 && current.Row.RowNumber() == result[len(result)-1].Row.RowNumber() {
			for i, v := range current.Values {
				result[len(result)-1].Values[i] = append(result[len(result)-1].Values[i], v...)
			}
			continue
		}

		result = append(result, current)
	}
	require.NoError(t, it.Err())
	return result
}

func int64ParquetComparer() cmp.Option {
	return cmp.Comparer(func(x, y parquet.Value) bool {
		return x.Int64() == y.Int64()
	})
}

type typedRepeatedTestSample struct {
	StacktraceID uint64 `parquet:",delta"`
	Value        int64  `parquet:",delta"`
	SpanID       uint64 `parquet:",optional"`
	TraceID      []byte `parquet:",optional"`
}

type typedRepeatedTestAnnotation struct {
	Key   string
	Value string
}

type typedRepeatedTestProfile struct {
	Samples     []typedRepeatedTestSample     `parquet:",list"`
	Annotations []typedRepeatedTestAnnotation `parquet:",list"`
}

var typedTestSchema = parquet.NewSchema("TypedTestProfile", parquet.Group{
	"Samples": parquet.List(parquet.Group{
		"StacktraceID": parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked),
		"Value":        parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked),
		"SpanID":       parquet.Optional(parquet.Encoded(parquet.Uint(64), &parquet.RLEDictionary)),
		"TraceID":      parquet.Optional(parquet.Encoded(parquet.Leaf(parquet.FixedLenByteArrayType(16)), &parquet.RLEDictionary)),
	}),
	"Annotations": parquet.List(parquet.Group{
		"Key":   parquet.String(),
		"Value": parquet.String(),
	}),
})

func Test_TypedRepeatedRowIterator(t *testing.T) {
	trace1 := model.TraceID{0x01, 0x02, 0x03, 15: 0xaa}
	trace2 := model.TraceID{0x10, 0x20, 0x30, 15: 0xbb}

	const numRows = 20 // > 2 * defaultBatchSize (4) to exercise buffer wraparound & pool reuse
	buf := parquet.NewBuffer(typedTestSchema)
	var rows []testRowGetter
	for r := 0; r < numRows; r++ {
		prof := typedRepeatedTestProfile{
			Samples: []typedRepeatedTestSample{
				{
					StacktraceID: uint64(r*10 + 1),
					Value:        int64(r*100 + 10),
					SpanID:       uint64(r + 1),
					TraceID:      trace1[:],
				},
				{
					StacktraceID: uint64(r*10 + 2),
					Value:        int64(r*100 + 20),
					SpanID:       0,
					TraceID:      nil, // null trace ID -> zero model.TraceID
				},
				{
					StacktraceID: uint64(r*10 + 3),
					Value:        int64(r*100 + 30),
					SpanID:       uint64(r + 2),
					TraceID:      trace2[:],
				},
			},
		}
		if r%2 == 0 {
			prof.Annotations = []typedRepeatedTestAnnotation{
				{Key: fmt.Sprintf("k1_%d", r), Value: fmt.Sprintf("v1_%d", r)},
				{Key: fmt.Sprintf("k2_%d", r), Value: fmt.Sprintf("v2_%d", r)},
			}
		}
		require.NoError(t, buf.Write(&prof))
		if r%3 != 1 { // skip some rows
			rows = append(rows, testRowGetter{RowNum: int64(r)})
		}
	}

	colIdx := func(path ...string) int {
		lc, ok := typedTestSchema.Lookup(path...)
		require.True(t, ok, "missing column %v", path)
		return lc.ColumnIndex
	}

	it := NewTypedRepeatedRowIteratorBatchSize(
		context.Background(),
		iter.NewSliceIterator(rows),
		[]parquet.RowGroup{buf},
		2, // small batch size to force frequent pool Get/Put swaps
	)
	defer func() {
		require.NoError(t, it.Close())
	}()

	stacktraces := it.Uint32Column(colIdx("Samples", "list", "element", "StacktraceID"))
	valuesU64 := it.Uint64Column(colIdx("Samples", "list", "element", "Value"))
	valuesI64 := it.Int64Column(colIdx("Samples", "list", "element", "Value"))
	spans := it.Uint64Column(colIdx("Samples", "list", "element", "SpanID"))
	traces := it.TraceIDColumn(colIdx("Samples", "list", "element", "TraceID"))
	annKeys := it.StringColumn(colIdx("Annotations", "list", "element", "Key"))
	annVals := it.StringColumn(colIdx("Annotations", "list", "element", "Value"))
	missingCol := it.StringColumn(-1)

	var seen int
	for it.Next() {
		r := int(it.At().RowNum)
		assert.Equal(t, rows[seen].RowNum, it.At().RowNum)

		assert.Equal(t, []uint32{uint32(r*10 + 1), uint32(r*10 + 2), uint32(r*10 + 3)}, stacktraces.At())
		assert.Equal(t, []uint64{uint64(r*100 + 10), uint64(r*100 + 20), uint64(r*100 + 30)}, valuesU64.At())
		assert.Equal(t, []int64{int64(r*100 + 10), int64(r*100 + 20), int64(r*100 + 30)}, valuesI64.At())
		assert.Equal(t, []uint64{uint64(r + 1), 0, uint64(r + 2)}, spans.At())
		assert.Equal(t, []model.TraceID{trace1, {}, trace2}, traces.At())
		assert.Nil(t, missingCol.At())

		if r%2 == 0 {
			assert.Equal(t, []string{fmt.Sprintf("k1_%d", r), fmt.Sprintf("k2_%d", r)}, annKeys.At())
			assert.Equal(t, []string{fmt.Sprintf("v1_%d", r), fmt.Sprintf("v2_%d", r)}, annVals.At())
		} else {
			assert.Empty(t, annKeys.At())
			assert.Empty(t, annVals.At())
		}
		seen++
	}
	require.NoError(t, it.Err())
	assert.Equal(t, len(rows), seen)
}

func Test_TypedRepeatedRowIterator_Cancellation(t *testing.T) {
	buf := parquet.NewBuffer(typedTestSchema)
	require.NoError(t, buf.Write(&typedRepeatedTestProfile{
		Samples: []typedRepeatedTestSample{{StacktraceID: 1, Value: 10}},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	col, ok := typedTestSchema.Lookup("Samples", "list", "element", "StacktraceID")
	require.True(t, ok)

	it := NewTypedRepeatedRowIterator(ctx, iter.NewSliceIterator([]testRowGetter{{0}}), []parquet.RowGroup{buf})
	_ = it.Uint32Column(col.ColumnIndex)

	assert.False(t, it.Next())
	assert.ErrorIs(t, it.Err(), context.Canceled)
	assert.NoError(t, it.Close())
}

func BenchmarkRepeatedRowIterator_TreeColumns(b *testing.B) {
	const (
		numRows       = 64
		samplesPerRow = 1024
	)
	buf := parquet.NewBuffer(typedTestSchema)
	rows := make([]testRowGetter, numRows)
	samples := make([]typedRepeatedTestSample, samplesPerRow)
	for r := 0; r < numRows; r++ {
		rows[r] = testRowGetter{RowNum: int64(r)}
		for s := 0; s < samplesPerRow; s++ {
			samples[s] = typedRepeatedTestSample{
				StacktraceID: uint64(s + 1),
				Value:        int64(s * 10),
			}
		}
		if err := buf.Write(&typedRepeatedTestProfile{Samples: samples}); err != nil {
			b.Fatal(err)
		}
	}
	rgs := []parquet.RowGroup{buf}
	sidCol, _ := typedTestSchema.Lookup("Samples", "list", "element", "StacktraceID")
	valCol, _ := typedTestSchema.Lookup("Samples", "list", "element", "Value")

	b.Run("ParquetValues", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			it := NewRepeatedRowIterator(context.Background(), iter.NewSliceIterator(rows), rgs, sidCol.ColumnIndex, valCol.ColumnIndex)
			var sum uint64
			for it.Next() {
				p := it.At()
				for j, sid := range p.Values[0] {
					if sid.Uint32() > 0 {
						sum += p.Values[1][j].Uint64()
					}
				}
			}
			if err := it.Err(); err != nil {
				b.Fatal(err)
			}
			_ = it.Close()
			_ = sum
		}
	})

	b.Run("TypedSlices", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			it := NewTypedRepeatedRowIterator(context.Background(), iter.NewSliceIterator(rows), rgs)
			sids := it.Uint32Column(sidCol.ColumnIndex)
			vals := it.Uint64Column(valCol.ColumnIndex)
			var sum uint64
			for it.Next() {
				st := sids.At()
				vs := vals.At()
				for j, sid := range st {
					if sid > 0 {
						sum += vs[j]
					}
				}
			}
			if err := it.Err(); err != nil {
				b.Fatal(err)
			}
			_ = it.Close()
			_ = sum
		}
	})
}

func BenchmarkRepeatedRowIterator_SpanTraceColumns(b *testing.B) {
	const (
		numRows       = 64
		samplesPerRow = 1024
	)
	trace := model.TraceID{0x01, 0x02, 0x03, 15: 0xff}
	buf := parquet.NewBuffer(typedTestSchema)
	rows := make([]testRowGetter, numRows)
	samples := make([]typedRepeatedTestSample, samplesPerRow)
	for r := 0; r < numRows; r++ {
		rows[r] = testRowGetter{RowNum: int64(r)}
		for s := 0; s < samplesPerRow; s++ {
			var tid []byte
			if s%2 == 0 {
				tid = trace[:]
			}
			samples[s] = typedRepeatedTestSample{
				StacktraceID: uint64(s + 1),
				Value:        int64(s * 10),
				SpanID:       uint64(s + 100),
				TraceID:      tid,
			}
		}
		if err := buf.Write(&typedRepeatedTestProfile{Samples: samples}); err != nil {
			b.Fatal(err)
		}
	}
	rgs := []parquet.RowGroup{buf}
	spanCol, _ := typedTestSchema.Lookup("Samples", "list", "element", "SpanID")
	valCol, _ := typedTestSchema.Lookup("Samples", "list", "element", "Value")
	traceCol, _ := typedTestSchema.Lookup("Samples", "list", "element", "TraceID")

	b.Run("ParquetValues", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			it := NewRepeatedRowIterator(context.Background(), iter.NewSliceIterator(rows), rgs, spanCol.ColumnIndex, valCol.ColumnIndex, traceCol.ColumnIndex)
			var sum int64
			for it.Next() {
				p := it.At()
				for j := range p.Values[0] {
					var tid model.TraceID
					if tb := p.Values[2][j].Bytes(); len(tb) == len(tid) {
						copy(tid[:], tb)
					}
					if p.Values[0][j].Uint64() > 0 && tid != (model.TraceID{}) {
						sum += p.Values[1][j].Int64()
					}
				}
			}
			if err := it.Err(); err != nil {
				b.Fatal(err)
			}
			_ = it.Close()
			_ = sum
		}
	})

	b.Run("TypedSlices", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			it := NewTypedRepeatedRowIterator(context.Background(), iter.NewSliceIterator(rows), rgs)
			spans := it.Uint64Column(spanCol.ColumnIndex)
			vals := it.Int64Column(valCol.ColumnIndex)
			traces := it.TraceIDColumn(traceCol.ColumnIndex)
			var sum int64
			for it.Next() {
				sp := spans.At()
				vs := vals.At()
				tr := traces.At()
				for j, spanID := range sp {
					if spanID > 0 && tr[j] != (model.TraceID{}) {
						sum += vs[j]
					}
				}
			}
			if err := it.Err(); err != nil {
				b.Fatal(err)
			}
			_ = it.Close()
			_ = sum
		}
	})
}
