package writer

import (
	"fmt"
	"io"

	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/segmentio/parquet-go"
)

type ValueWriter interface {
	WritePage(p parquet.Page) error
	Write([]parquet.Value)
}

type binaryValueWriter struct {
	b          *array.BinaryBuilder
	numValues  int
	firstWrite bool
}

func NewBinaryValueWriter(b array.Builder, numValues int) ValueWriter {
	return &binaryValueWriter{
		b:          b.(*array.BinaryBuilder),
		numValues:  numValues,
		firstWrite: true,
	}
}

func (w *binaryValueWriter) Write(values []parquet.Value) {
	if w.firstWrite {
		w.firstWrite = false

		// Depending on the nullability of the column this could be optimized
		// further by reading strings directly and adding all of them at once
		// to the array builder.
		vs := make([][]byte, len(values))
		validity := make([]bool, len(values))
		largest := 0
		for i, v := range values {
			if !v.IsNull() {
				vs[i] = v.ByteArray()
				if len(vs[i]) > largest {
					largest = len(vs[i])
				}
				validity[i] = true
			}
		}
		w.b.ReserveData(w.numValues * largest)

		w.b.AppendValues(vs, validity)
	} else {
		// Depending on the nullability of the column this could be optimized
		// further by reading strings directly and adding all of them at once
		// to the array builder.
		vs := make([][]byte, len(values))
		validity := make([]bool, len(values))
		for i, v := range values {
			if !v.IsNull() {
				vs[i] = v.ByteArray()
				validity[i] = true
			}
		}

		w.b.AppendValues(vs, validity)
	}
}

// TODO: implement fast path of writing the whole page directly.
func (w *binaryValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type int64ValueWriter struct {
	b   *array.Int64Builder
	buf []int64
}

func NewInt64ValueWriter(b array.Builder, numValues int) ValueWriter {
	res := &int64ValueWriter{
		b: b.(*array.Int64Builder),
	}
	res.b.Reserve(numValues)
	return res
}

func (w *int64ValueWriter) Write(values []parquet.Value) {
	// Depending on the nullability of the column this could be optimized
	// further by reading int64s directly and adding all of them at once to
	// the array builder.
	for _, v := range values {
		if v.IsNull() {
			w.b.AppendNull()
		} else {
			w.b.Append(v.Int64())
		}
	}
}

func (w *int64ValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	ireader, ok := reader.(parquet.Int64Reader)
	if ok {
		// fast path
		if w.buf == nil {
			w.buf = make([]int64, p.NumValues())
		}
		values := w.buf
		for {
			n, err := ireader.ReadInt64s(values)
			if err != nil && err != io.EOF {
				return fmt.Errorf("read values: %w", err)
			}

			w.b.AppendValues(values[:n], nil)
			if err == io.EOF {
				break
			}
		}
		return nil
	}

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type uint64ValueWriter struct {
	b *array.Uint64Builder
}

func NewUint64ValueWriter(b array.Builder, numValues int) ValueWriter {
	res := &uint64ValueWriter{
		b: b.(*array.Uint64Builder),
	}
	res.b.Reserve(numValues)
	return res
}

func (w *uint64ValueWriter) Write(values []parquet.Value) {
	// Depending on the nullability of the column this could be optimized
	// further by reading uint64s directly and adding all of them at once
	// to the array builder.
	for _, v := range values {
		if v.IsNull() {
			w.b.AppendNull()
		} else {
			w.b.Append(uint64(v.Int64()))
		}
	}
}

// TODO: implement fast path of writing the whole page directly.
func (w *uint64ValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}

type repeatedValueWriter struct {
	b      *array.ListBuilder
	values ValueWriter
}

func NewListValueWriter(newValueWriter func(b array.Builder, numValues int) ValueWriter) func(b array.Builder, numValues int) ValueWriter {
	return func(b array.Builder, numValues int) ValueWriter {
		builder := b.(*array.ListBuilder)

		return &repeatedValueWriter{
			b:      builder,
			values: newValueWriter(builder.ValueBuilder(), numValues),
		}
	}
}

func (w *repeatedValueWriter) Write(values []parquet.Value) {
	v0 := values[0]
	rep := v0.RepetitionLevel()
	def := v0.DefinitionLevel()
	if rep == 0 && def == 0 {
		w.b.AppendNull()
	}

	w.b.Append(true)
	w.values.Write(values)
}

// TODO: implement fast path of writing the whole page directly.
func (w *repeatedValueWriter) WritePage(p parquet.Page) error {
	reader := p.Values()

	values := make([]parquet.Value, p.NumValues())
	_, err := reader.ReadValues(values)
	// We're reading all values in the page so we always expect an io.EOF.
	if err != nil && err != io.EOF {
		return fmt.Errorf("read values: %w", err)
	}

	w.Write(values)

	return nil
}
