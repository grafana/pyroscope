package parquet

import (
	"io"

	"github.com/segmentio/parquet-go/deprecated"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/internal/bits"
)

type optionalPageValues struct {
	page   *optionalPage
	values ValueReader
	offset int
}

func (r *optionalPageValues) ReadValues(values []Value) (n int, err error) {
	maxDefinitionLevel := r.page.maxDefinitionLevel
	columnIndex := ^int16(r.page.Column())

	for n < len(values) && r.offset < len(r.page.definitionLevels) {
		for n < len(values) && r.offset < len(r.page.definitionLevels) && r.page.definitionLevels[r.offset] != maxDefinitionLevel {
			values[n] = Value{
				definitionLevel: r.page.definitionLevels[r.offset],
				columnIndex:     columnIndex,
			}
			r.offset++
			n++
		}

		i := n
		j := r.offset
		for i < len(values) && j < len(r.page.definitionLevels) && r.page.definitionLevels[j] == maxDefinitionLevel {
			i++
			j++
		}

		if n < i {
			for j, err = r.values.ReadValues(values[n:i]); j > 0; j-- {
				values[n].definitionLevel = maxDefinitionLevel
				r.offset++
				n++
			}
			// Do not return on an io.EOF here as we may still have null values to read.
			if err != nil && err != io.EOF {
				return n, err
			}
			err = nil
		}
	}

	if r.offset == len(r.page.definitionLevels) {
		err = io.EOF
	}
	return n, err
}

type repeatedPageValues struct {
	page   *repeatedPage
	values ValueReader
	offset int
}

func (r *repeatedPageValues) ReadValues(values []Value) (n int, err error) {
	maxDefinitionLevel := r.page.maxDefinitionLevel
	columnIndex := ^int16(r.page.Column())

	for n < len(values) && r.offset < len(r.page.definitionLevels) {
		for n < len(values) && r.offset < len(r.page.definitionLevels) && r.page.definitionLevels[r.offset] != maxDefinitionLevel {
			values[n] = Value{
				repetitionLevel: r.page.repetitionLevels[r.offset],
				definitionLevel: r.page.definitionLevels[r.offset],
				columnIndex:     columnIndex,
			}
			r.offset++
			n++
		}

		i := n
		j := r.offset
		for i < len(values) && j < len(r.page.definitionLevels) && r.page.definitionLevels[j] == maxDefinitionLevel {
			i++
			j++
		}

		if n < i {
			for j, err = r.values.ReadValues(values[n:i]); j > 0; j-- {
				values[n].repetitionLevel = r.page.repetitionLevels[r.offset]
				values[n].definitionLevel = maxDefinitionLevel
				r.offset++
				n++
			}
			if err != nil && err != io.EOF {
				return n, err
			}
			err = nil
		}
	}

	if r.offset == len(r.page.definitionLevels) {
		err = io.EOF
	}
	return n, err
}

type booleanPageValues struct {
	page   *booleanPage
	offset int
}

func (r *booleanPageValues) ReadBooleans(values []bool) (n int, err error) {
	for n < len(values) && r.offset < int(r.page.numValues) {
		values[n] = r.page.valueAt(r.offset)
		r.offset++
		n++
	}
	if r.offset == int(r.page.numValues) {
		err = io.EOF
	}
	return n, err
}

func (r *booleanPageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < int(r.page.numValues) {
		values[n] = makeValueBoolean(r.page.valueAt(r.offset))
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == int(r.page.numValues) {
		err = io.EOF
	}
	return n, err
}

type int32PageValues struct {
	page   *int32Page
	offset int
}

func (r *int32PageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadInt32s(bits.BytesToInt32(b))
	return 4 * n, err
}

func (r *int32PageValues) ReadInt32s(values []int32) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *int32PageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueInt32(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type int64PageValues struct {
	page   *int64Page
	offset int
}

func (r *int64PageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadInt64s(bits.BytesToInt64(b))
	return 8 * n, err
}

func (r *int64PageValues) ReadInt64s(values []int64) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *int64PageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueInt64(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type int96PageValues struct {
	page   *int96Page
	offset int
}

func (r *int96PageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadInt96s(deprecated.BytesToInt96(b))
	return 12 * n, err
}

func (r *int96PageValues) ReadInt96s(values []deprecated.Int96) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *int96PageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueInt96(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type floatPageValues struct {
	page   *floatPage
	offset int
}

func (r *floatPageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadFloats(bits.BytesToFloat32(b))
	return 4 * n, err
}

func (r *floatPageValues) ReadFloats(values []float32) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *floatPageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueFloat(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type doublePageValues struct {
	page   *doublePage
	offset int
}

func (r *doublePageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadDoubles(bits.BytesToFloat64(b))
	return 8 * n, err
}

func (r *doublePageValues) ReadDoubles(values []float64) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *doublePageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueDouble(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type byteArrayPageValues struct {
	page   *byteArrayPage
	offset int
}

func (r *byteArrayPageValues) Read(b []byte) (int, error) {
	_, n, err := r.readByteArrays(b)
	return n, err
}

func (r *byteArrayPageValues) ReadRequired(values []byte) (int, error) {
	return r.ReadByteArrays(values)
}

func (r *byteArrayPageValues) ReadByteArrays(values []byte) (int, error) {
	n, _, err := r.readByteArrays(values)
	return n, err
}

func (r *byteArrayPageValues) readByteArrays(values []byte) (c, n int, err error) {
	for r.offset < len(r.page.values) {
		b := r.page.valueAt(uint32(r.offset))
		k := plain.ByteArrayLengthSize + len(b)
		if k > (len(values) - n) {
			break
		}
		plain.PutByteArrayLength(values[n:], len(b))
		n += plain.ByteArrayLengthSize
		n += copy(values[n:], b)
		r.offset += plain.ByteArrayLengthSize
		r.offset += len(b)
		c++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	} else if n == 0 && len(values) > 0 {
		err = io.ErrShortBuffer
	}
	return c, n, err
}

func (r *byteArrayPageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		value := r.page.valueAt(uint32(r.offset))
		values[n] = makeValueBytes(ByteArray, value)
		values[n].columnIndex = r.page.columnIndex
		r.offset += plain.ByteArrayLengthSize
		r.offset += len(value)
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type fixedLenByteArrayPageValues struct {
	page   *fixedLenByteArrayPage
	offset int
}

func (r *fixedLenByteArrayPageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadFixedLenByteArrays(b)
	return n * r.page.size, err
}

func (r *fixedLenByteArrayPageValues) ReadRequired(values []byte) (int, error) {
	return r.ReadFixedLenByteArrays(values)
}

func (r *fixedLenByteArrayPageValues) ReadFixedLenByteArrays(values []byte) (n int, err error) {
	n = copy(values, r.page.data[r.offset:]) / r.page.size
	r.offset += n * r.page.size
	if r.offset == len(r.page.data) {
		err = io.EOF
	} else if n == 0 && len(values) > 0 {
		err = io.ErrShortBuffer
	}
	return n, err
}

func (r *fixedLenByteArrayPageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.data) {
		values[n] = makeValueBytes(FixedLenByteArray, r.page.data[r.offset:r.offset+r.page.size])
		values[n].columnIndex = r.page.columnIndex
		r.offset += r.page.size
		n++
	}
	if r.offset == len(r.page.data) {
		err = io.EOF
	}
	return n, err
}

type uint32PageValues struct {
	page   *uint32Page
	offset int
}

func (r *uint32PageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadUint32s(bits.BytesToUint32(b))
	return 4 * n, err
}

func (r *uint32PageValues) ReadUint32s(values []uint32) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *uint32PageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueUint32(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type uint64PageValues struct {
	page   *uint64Page
	offset int
}

func (r *uint64PageValues) Read(b []byte) (n int, err error) {
	n, err = r.ReadUint64s(bits.BytesToUint64(b))
	return 8 * n, err
}

func (r *uint64PageValues) ReadUint64s(values []uint64) (n int, err error) {
	n = copy(values, r.page.values[r.offset:])
	r.offset += n
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

func (r *uint64PageValues) ReadValues(values []Value) (n int, err error) {
	for n < len(values) && r.offset < len(r.page.values) {
		values[n] = makeValueUint64(r.page.values[r.offset])
		values[n].columnIndex = r.page.columnIndex
		r.offset++
		n++
	}
	if r.offset == len(r.page.values) {
		err = io.EOF
	}
	return n, err
}

type nullPageValues struct {
	column int
	remain int
}

func (r *nullPageValues) ReadValues(values []Value) (n int, err error) {
	columnIndex := ^int16(r.column)
	values = values[:min(r.remain, len(values))]
	for i := range values {
		values[i] = Value{columnIndex: columnIndex}
	}
	r.remain -= len(values)
	if r.remain == 0 {
		err = io.EOF
	}
	return len(values), err
}
